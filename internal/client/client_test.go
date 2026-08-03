package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient points a Client at srv and shortens the backoff so retry tests
// finish in milliseconds.
func newTestClient(srv *httptest.Server) *Client {
	c := New(srv.URL, "lin_api_test")
	c.backoffBase = time.Millisecond
	return c
}

func TestQuerySendsDocumentAndVariables(t *testing.T) {
	var gotAuth, gotContentType string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, `{"data":{"team":{"id":"t1","name":"Engineering"}}}`)
	}))
	defer srv.Close()

	var out struct {
		Team struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
	}
	err := newTestClient(srv).Query(context.Background(), `query team($id: String!) { team(id: $id) { id name } }`,
		map[string]any{"id": "t1"}, &out)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Linear rejects a "Bearer " prefix — the key goes over verbatim.
	if gotAuth != "lin_api_test" {
		t.Errorf("Authorization = %q, want the bare key", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if q, _ := gotBody["query"].(string); !strings.Contains(q, "team(id: $id)") {
		t.Errorf("query not sent: %v", gotBody["query"])
	}
	vars, _ := gotBody["variables"].(map[string]any)
	if vars["id"] != "t1" {
		t.Errorf("variables = %v", gotBody["variables"])
	}
	if out.Team.Name != "Engineering" {
		t.Errorf("data not decoded: %+v", out.Team)
	}
}

func TestMutateOmitsEmptyVariables(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, `{"data":{"ok":true}}`)
	}))
	defer srv.Close()

	if err := newTestClient(srv).Mutate(context.Background(), `mutation ping { ping }`, nil, nil); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if _, ok := gotBody["variables"]; ok {
		t.Errorf("variables should be omitted when empty, got %v", gotBody["variables"])
	}
}

// A GraphQL failure arrives as HTTP 200 with a populated errors[] — the trap
// this whole client exists to handle.
func TestErrorsInsideHTTP200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errors":[{"message":"Argument Validation Error",
			"extensions":{"type":"InvalidInput","userPresentableMessage":"Name is required"}}],"data":null}`)
	}))
	defer srv.Close()

	err := newTestClient(srv).Mutate(context.Background(), `mutation teamCreate { teamCreate { success } }`, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a populated errors[] inside a 200")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
	if !apiErr.HasType("InvalidInput") {
		t.Errorf("HasType(InvalidInput) false for %v", apiErr.Errors)
	}
	if !strings.Contains(apiErr.Error(), "teamCreate") {
		t.Errorf("error should name the operation: %s", apiErr.Error())
	}
	if NotFound(err) {
		t.Error("NotFound must not match an InvalidInput error")
	}
}

func TestNotFoundKeysOffEntityNotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Note the 200: keying NotFound off the status code would miss this, and a
		// deleted resource would never leave the state.
		_, _ = io.WriteString(w, `{"errors":[{"message":"Entity not found",
			"extensions":{"type":"EntityNotFoundError","code":"NOT_FOUND"}}],"data":null}`)
	}))
	defer srv.Close()

	err := newTestClient(srv).Query(context.Background(), `query issueLabel { issueLabel(id: "gone") { id } }`, nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !NotFound(err) {
		t.Errorf("NotFound = false for %v", err)
	}
}

func TestNotFoundIgnoresUnrelatedErrors(t *testing.T) {
	if NotFound(nil) {
		t.Error("NotFound(nil) = true")
	}
	if NotFound(errors.New("dial tcp: connection refused")) {
		t.Error("NotFound matched a transport error")
	}
}

func TestRetriesRateLimitHonouringRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0.01")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"errors":[{"message":"Ratelimited"}]}`)
			return
		}
		w.Header().Set("X-RateLimit-Requests-Remaining", "1420")
		_, _ = io.WriteString(w, `{"data":{"viewer":{"id":"u1"}}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var out struct {
		Viewer struct {
			ID string `json:"id"`
		} `json:"viewer"`
	}
	if err := c.Query(context.Background(), `query viewer { viewer { id } }`, nil, &out); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 (one 429, one success)", calls.Load())
	}
	if out.Viewer.ID != "u1" {
		t.Errorf("retry did not decode the eventual success: %+v", out)
	}
	remaining, ok := c.RequestsRemaining()
	if !ok || remaining != 1420 {
		t.Errorf("RequestsRemaining = (%d, %v), want (1420, true)", remaining, ok)
	}
}

// A rate limit can also arrive as errors[] inside a 200 — same treatment.
func TestRetriesRatelimitedErrorInsideHTTP200(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0.01")
			_, _ = io.WriteString(w, `{"errors":[{"message":"Ratelimited",
				"extensions":{"type":"RatelimitedError"}}],"data":null}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"viewer":{"id":"u1"}}}`)
	}))
	defer srv.Close()

	if err := newTestClient(srv).Query(context.Background(), `query viewer { viewer { id } }`, nil, nil); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestRetriesTransient5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"viewer":{"id":"u1"}}}`)
	}))
	defer srv.Close()

	if err := newTestClient(srv).Query(context.Background(), `query viewer { viewer { id } }`, nil, nil); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3 (two 502s, one success)", calls.Load())
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `upstream unavailable`)
	}))
	defer srv.Close()

	err := newTestClient(srv).Query(context.Background(), `query viewer { viewer { id } }`, nil, nil)
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if int(calls.Load()) != maxAttempts {
		t.Errorf("calls = %d, want %d", calls.Load(), maxAttempts)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected a 503 *APIError, got %v", err)
	}
}

func TestNon2xxCarriesGraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errors":[{"message":"Authentication required, not authenticated",
			"extensions":{"type":"AuthenticationError"}}]}`)
	}))
	defer srv.Close()

	err := newTestClient(srv).Query(context.Background(), `query viewer { viewer { id } }`, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || !apiErr.HasType("AuthenticationError") {
		t.Errorf("unexpected error: %v (%d)", apiErr, apiErr.StatusCode)
	}
}

func TestContextCancellationStopsRetryLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := newTestClient(srv).Query(ctx, `query viewer { viewer { id } }`, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the retry wait to honour the context, got %v", err)
	}
}

func TestOperationName(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{`query team($id: String!) { team(id: $id) { id } }`, "team"},
		{`mutation teamCreate($input: TeamCreateInput!) { teamCreate(input: $input) { success } }`, "teamCreate"},
		{"query workflowStates {\n  workflowStates { nodes { id } }\n}", "workflowStates"},
		{`{ viewer { id } }`, "graphql"},
		{`query { viewer { id } }`, "graphql"},
	} {
		if got := operationName(tc.doc); got != tc.want {
			t.Errorf("operationName(%q) = %q, want %q", tc.doc, got, tc.want)
		}
	}
}

func TestNewDefaultsEndpoint(t *testing.T) {
	if got := New("", "k").endpoint; got != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", got, DefaultEndpoint)
	}
	if got := New("https://example.test/graphql/", "k").endpoint; got != "https://example.test/graphql" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
}
