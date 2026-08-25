// Package client is a minimal client for the Linear GraphQL API.
//
// Shape of the API this talks to — it differs from a REST API in every way that
// matters here:
//   - A single endpoint, https://api.linear.app/graphql. There are no
//     per-resource paths, so this client exposes Query/Mutate rather than
//     Get/List/Write/Delete. Every call is a POST.
//   - Auth: Authorization: <api key>. Linear rejects a "Bearer " prefix. A
//     personal API key is scoped to one workspace.
//   - Failures arrive as HTTP 200 with a populated errors[] array. The 404
//     equivalent is extensions.type == "EntityNotFoundError", which is what
//     NotFound keys off — never the status code. Get that wrong and a Read never
//     drops a deleted resource from state, so the next plan dies at refresh.
//   - Rate limiting is 1500 requests/hour per key, reported through
//     X-RateLimit-Requests-Remaining. A 429 carries Retry-After; the client
//     honours it (and retries transient 5xx with backoff) transparently, so
//     resources never see either.
//   - A rejected mutation input comes back as "Argument Validation Error", a
//     message that names neither the field nor the rule. What names them is
//     extensions.validationErrors, so APIError renders that alongside the
//     message — without it the operator learns only that something was invalid,
//     and only at apply time, since Terraform's own plan accepts any string.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultEndpoint is Linear's GraphQL endpoint.
const DefaultEndpoint = "https://api.linear.app/graphql"

// maxAttempts bounds the retry loop for 429 / 5xx responses.
const maxAttempts = 6

// entityNotFound is the extensions.type Linear reports for a missing entity —
// the GraphQL equivalent of a 404, delivered inside an HTTP 200.
const entityNotFound = "EntityNotFoundError"

// rateLimitedType is the extensions.type Linear reports when the hourly request
// budget is exhausted. It can arrive inside a 200 as well as alongside a 429.
const rateLimitedType = "RatelimitedError"

// Client talks to the Linear GraphQL API with a personal API key.
type Client struct {
	httpClient *http.Client
	endpoint   string
	token      string
	userAgent  string

	// backoffBase is the first 5xx retry wait, doubling per attempt. A field
	// rather than a constant so the tests can drive the retry loop without
	// sleeping for real seconds.
	backoffBase time.Duration

	// remaining caches the last X-RateLimit-Requests-Remaining value seen, for
	// diagnostics. Stored as value+1 so zero means "never seen".
	remaining atomic.Int64
}

// New constructs a Client. An empty endpoint falls back to DefaultEndpoint.
func New(endpoint, token string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		endpoint:   strings.TrimRight(endpoint, "/"),
		token:      token,
		userAgent:  "terraform-provider-linear (+https://github.com/kirchDev/terraform-provider-linear)",

		backoffBase: time.Second,
	}
}

// Query executes a GraphQL query document and, when out is non-nil, unmarshals
// the "data" object into it.
func (c *Client) Query(ctx context.Context, doc string, vars map[string]any, out any) error {
	return c.execute(ctx, doc, vars, out)
}

// Mutate executes a GraphQL mutation document and, when out is non-nil,
// unmarshals the "data" object into it. Transport-wise it is identical to
// Query — the split exists so calling code reads as create/read/update/delete.
func (c *Client) Mutate(ctx context.Context, doc string, vars map[string]any, out any) error {
	return c.execute(ctx, doc, vars, out)
}

// RequestsRemaining reports the hourly budget left according to the last
// response seen, and whether that header has been observed at all.
func (c *Client) RequestsRemaining() (int64, bool) {
	v := c.remaining.Load()
	if v == 0 {
		return 0, false
	}
	return v - 1, true
}

// NotFound reports whether err carries Linear's EntityNotFoundError, so a Read
// can drop the resource from state instead of failing the plan.
func NotFound(err error) bool {
	var e *APIError
	if errors.As(err, &e) {
		return e.HasType(entityNotFound)
	}
	return false
}

// GraphQLError is one entry of a GraphQL errors[] array.
type GraphQLError struct {
	Message   string         `json:"message"`
	Path      []any          `json:"path"`
	Extension map[string]any `json:"-"`
}

// UnmarshalJSON keeps extensions as a free-form map — Linear puts the parts that
// matter (type, code, userPresentableMessage) in there, and the set grows.
func (e *GraphQLError) UnmarshalJSON(data []byte) error {
	var raw struct {
		Message    string         `json:"message"`
		Path       []any          `json:"path"`
		Extensions map[string]any `json:"extensions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Message = raw.Message
	e.Path = raw.Path
	e.Extension = raw.Extensions
	return nil
}

// Type returns extensions.type, empty when absent.
func (e *GraphQLError) Type() string {
	s, _ := e.Extension["type"].(string)
	return s
}

// ValidationError is one entry of extensions.validationErrors: the input
// property Linear rejected, and the constraints it broke keyed by rule name.
type ValidationError struct {
	Property    string
	Constraints map[string]string
}

// String renders the entry as "property: message (rule)". Constraints are
// ordered by rule name so one rejection always reads the same way.
func (v ValidationError) String() string {
	if len(v.Constraints) == 0 {
		return v.Property
	}
	rules := make([]string, 0, len(v.Constraints))
	for rule := range v.Constraints {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	msgs := make([]string, 0, len(rules))
	for _, rule := range rules {
		msgs = append(msgs, fmt.Sprintf("%s (%s)", v.Constraints[rule], rule))
	}
	return v.Property + ": " + strings.Join(msgs, "; ")
}

// ValidationErrors reads extensions.validationErrors, which Linear populates
// when it rejects a mutation input. Anything absent or of an unexpected shape is
// skipped rather than guessed at, so an error carrying no usable entry renders
// exactly as it did before this was read at all.
func (e *GraphQLError) ValidationErrors() []ValidationError {
	raw, ok := e.Extension["validationErrors"].([]any)
	if !ok {
		return nil
	}
	out := make([]ValidationError, 0, len(raw))
	for _, entry := range raw {
		fields, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		property, _ := fields["property"].(string)
		if property == "" {
			continue
		}
		ve := ValidationError{Property: property}
		if constraints, ok := fields["constraints"].(map[string]any); ok {
			ve.Constraints = make(map[string]string, len(constraints))
			for rule, msg := range constraints {
				if s, ok := msg.(string); ok {
					ve.Constraints[rule] = s
				}
			}
		}
		out = append(out, ve)
	}
	return out
}

// APIError is a failed GraphQL call: either a non-2xx transport response or —
// far more commonly — an HTTP 200 carrying a populated errors[].
type APIError struct {
	StatusCode int
	Operation  string
	Errors     []GraphQLError
	Body       string
}

func (e *APIError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("linear API %s: status %d: %s", e.Operation, e.StatusCode, e.Body)
	}
	parts := make([]string, 0, len(e.Errors))
	for _, ge := range e.Errors {
		part := ge.Message
		if t := ge.Type(); t != "" {
			part = fmt.Sprintf("%s: %s", t, ge.Message)
		}
		// Indented on their own lines: the message above says only that
		// something was invalid, these say what and why.
		for _, ve := range ge.ValidationErrors() {
			part += "\n  " + ve.String()
		}
		parts = append(parts, part)
	}
	return fmt.Sprintf("linear API %s: %s", e.Operation, strings.Join(parts, "; "))
}

// HasType reports whether any of the GraphQL errors carries extensions.type t.
func (e *APIError) HasType(t string) bool {
	for _, ge := range e.Errors {
		if ge.Type() == t {
			return true
		}
	}
	return false
}

// response is the envelope every GraphQL reply arrives in.
type response struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors"`
}

func (c *Client) execute(ctx context.Context, doc string, vars map[string]any, out any) error {
	payload := map[string]any{"query": doc}
	if len(vars) > 0 {
		payload["variables"] = vars
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	op := operationName(doc)

	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		// Verbatim, no "Bearer " prefix — Linear rejects the prefix.
		req.Header.Set("Authorization", c.token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil {
			return readErr
		}
		c.recordRateLimit(res)

		// Rate limited: wait out Retry-After and try again.
		if res.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts {
			if err := sleepCtx(ctx, retryAfter(res)); err != nil {
				return err
			}
			continue
		}

		// Transient server errors: exponential backoff and retry.
		if res.StatusCode >= 500 && attempt < maxAttempts {
			if err := sleepCtx(ctx, backoff(c.backoffBase, attempt)); err != nil {
				return err
			}
			continue
		}

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			apiErr := &APIError{StatusCode: res.StatusCode, Operation: op, Body: strings.TrimSpace(string(data))}
			var envelope response
			if json.Unmarshal(data, &envelope) == nil {
				apiErr.Errors = envelope.Errors
			}
			return apiErr
		}

		var envelope response
		if err := json.Unmarshal(data, &envelope); err != nil {
			return fmt.Errorf("decoding %s response: %w", op, err)
		}

		// The whole point of this client: a 200 can still be a failure.
		if len(envelope.Errors) > 0 {
			apiErr := &APIError{
				StatusCode: res.StatusCode,
				Operation:  op,
				Errors:     envelope.Errors,
				Body:       strings.TrimSpace(string(data)),
			}
			// A rate limit reported inside a 200 deserves the same retry as a 429.
			if apiErr.HasType(rateLimitedType) && attempt < maxAttempts {
				if err := sleepCtx(ctx, retryAfter(res)); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}

		if out != nil && len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, out); err != nil {
				return fmt.Errorf("decoding %s data: %w", op, err)
			}
		}
		return nil
	}
}

func (c *Client) recordRateLimit(res *http.Response) {
	h := res.Header.Get("X-RateLimit-Requests-Remaining")
	if h == "" {
		return
	}
	n, err := strconv.ParseInt(h, 10, 64)
	if err != nil {
		return
	}
	c.remaining.Store(n + 1)
}

// operationName extracts the operation name from a GraphQL document, for error
// messages. Falls back to "graphql" for anonymous documents.
func operationName(doc string) string {
	fields := strings.Fields(doc)
	for i, f := range fields {
		if f != "query" && f != "mutation" {
			continue
		}
		if i+1 >= len(fields) {
			break
		}
		name := fields[i+1]
		if idx := strings.IndexAny(name, "({"); idx >= 0 {
			name = name[:idx]
		}
		if name != "" {
			return name
		}
		break
	}
	return "graphql"
}

// retryAfter derives how long to wait before retrying a rate-limited request.
func retryAfter(res *http.Response) time.Duration {
	if h := res.Header.Get("Retry-After"); h != "" {
		if secs, err := strconv.ParseFloat(h, 64); err == nil && secs > 0 {
			return time.Duration(secs * float64(time.Second))
		}
	}
	return time.Second
}

// backoff returns the wait before the attempt-th retry of a 5xx (base, 2×base,
// 4×base, …, capped at 30s).
func backoff(base time.Duration, attempt int) time.Duration {
	d := time.Duration(math.Pow(2, float64(attempt-1))) * base
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// sleepCtx waits for d, returning early if ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
