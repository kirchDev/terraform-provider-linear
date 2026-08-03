package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// Acceptance tests run against an in-memory stand-in for Linear rather than a
// real workspace: they need no token, no network and no cleanup, and they can
// assert on things a live workspace cannot be made to do on demand — a filter
// coming back re-serialised, say.
//
// linearMock leans on how regular Linear's API is. Every entity is created with
// xCreate(input:), read with x(id:), updated with xUpdate(id:, input:) and
// removed with xDelete(id:) or xArchive(id:), so one handler covers all of them
// without a per-entity mock.

type linearMock struct {
	mu sync.Mutex
	// entities maps entity name → id → stored fields, in the shape a read
	// returns them.
	entities map[string]map[string]map[string]any
	nextID   int

	// normaliseJSON re-serialises JSON scalars on the way in, standing in for
	// Linear's server-side normalisation of filterData and friends.
	normaliseJSON bool
}

func newLinearMock() *linearMock {
	return &linearMock{
		entities:      map[string]map[string]map[string]any{},
		normaliseJSON: true,
	}
}

func (m *linearMock) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(srv.Close)
	return srv
}

type mockRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func (m *linearMock) handle(w http.ResponseWriter, r *http.Request) {
	// Linear takes the API key verbatim; a Bearer prefix would be rejected, and
	// asserting that here keeps the client honest.
	if auth := r.Header.Get("Authorization"); auth == "" || strings.HasPrefix(auth, "Bearer ") {
		m.writeError(w, "AuthenticationError", "Authentication required, not authenticated")
		return
	}

	raw, _ := io.ReadAll(r.Body)
	var req mockRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		m.writeError(w, "InvalidInput", "malformed request body")
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	op := mockOperationName(req.Query)
	switch {
	case strings.HasSuffix(op, "Create"):
		m.create(w, strings.TrimSuffix(op, "Create"), req)
	case strings.HasSuffix(op, "Update"):
		m.update(w, strings.TrimSuffix(op, "Update"), req)
	case strings.HasSuffix(op, "Delete"):
		m.remove(w, strings.TrimSuffix(op, "Delete"), req, "Delete")
	case strings.HasSuffix(op, "Archive"):
		m.remove(w, strings.TrimSuffix(op, "Archive"), req, "Archive")
	default:
		m.read(w, op, req)
	}
}

// mockOperationName pulls the operation name out of a GraphQL document. Every
// document this provider sends is named, and the name is what tells the mock
// which entity and which verb it is being asked for.
func mockOperationName(doc string) string {
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
		return name
	}
	return ""
}

func (m *linearMock) create(w http.ResponseWriter, name string, req mockRequest) {
	input, _ := req.Variables["input"].(map[string]any)

	m.nextID++
	id := fmt.Sprintf("00000000-0000-4000-8000-%012d", m.nextID)

	stored := m.store(name, id, input, nil)
	m.writeData(w, map[string]any{
		name + "Create": map[string]any{"success": true, name: stored},
	})
}

func (m *linearMock) update(w http.ResponseWriter, name string, req mockRequest) {
	id, _ := req.Variables["id"].(string)
	existing := m.entities[name][id]
	if existing == nil {
		m.writeError(w, "EntityNotFoundError", name+" not found")
		return
	}
	input, _ := req.Variables["input"].(map[string]any)

	stored := m.store(name, id, input, existing)
	m.writeData(w, map[string]any{
		name + "Update": map[string]any{"success": true, name: stored},
	})
}

func (m *linearMock) read(w http.ResponseWriter, name string, req mockRequest) {
	id, _ := req.Variables["id"].(string)
	stored := m.entities[name][id]
	if stored == nil {
		// The trap this provider is built around: a missing entity is an HTTP 200
		// with errors[], not a 404.
		m.writeError(w, "EntityNotFoundError", name+" not found")
		return
	}
	m.writeData(w, map[string]any{name: stored})
}

func (m *linearMock) remove(w http.ResponseWriter, name string, req mockRequest, verb string) {
	id, _ := req.Variables["id"].(string)
	delete(m.entities[name], id)
	m.writeData(w, map[string]any{name + verb: map[string]any{"success": true}})
}

// store merges an input object into the stored entity, translating the input's
// shape into the read shape: Linear takes `teamId` on the way in and returns
// `team { id }` on the way out.
func (m *linearMock) store(name, id string, input, existing map[string]any) map[string]any {
	stored := map[string]any{}
	for k, v := range existing {
		stored[k] = v
	}
	stored["id"] = id

	for key, value := range input {
		if value == nil {
			delete(stored, key)
			delete(stored, strings.TrimSuffix(key, "Id"))
			continue
		}

		if relation, ok := strings.CutSuffix(key, "Id"); ok && relation != "" {
			if s, isString := value.(string); isString {
				stored[relation] = map[string]any{"id": s}
				// Some Linear types expose the scalar id as well as the relation.
				stored[key] = s
				continue
			}
		}

		if m.normaliseJSON {
			if obj, ok := value.(map[string]any); ok {
				// Round-tripping through Go's encoder sorts the keys, which is the
				// point: state that survives it proves the comparison is semantic.
				if reencoded, err := json.Marshal(obj); err == nil {
					var back any
					if json.Unmarshal(reencoded, &back) == nil {
						stored[key] = back
						continue
					}
				}
			}
		}
		stored[key] = value
	}

	if m.entities[name] == nil {
		m.entities[name] = map[string]map[string]any{}
	}
	m.entities[name][id] = stored
	return stored
}

func (m *linearMock) writeData(w http.ResponseWriter, data map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Requests-Remaining", "1400")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func (m *linearMock) writeError(w http.ResponseWriter, typ, message string) {
	w.Header().Set("Content-Type", "application/json")
	// Deliberately a 200: this is how Linear reports failures.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": nil,
		"errors": []map[string]any{{
			"message":    message,
			"extensions": map[string]any{"type": typ},
		}},
	})
}

// count reports how many entities of a kind the mock holds, for assertions that
// a destroy actually deleted something.
func (m *linearMock) count(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entities[name])
}

// protoV6ProviderFactories serves the provider under test. Which Linear it
// talks to is decided by the endpoint in the provider block, not here.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"linear": providerserver.NewProtocol6WithError(New("test")()),
}

// providerConfig is the provider block every acceptance test starts from.
func providerConfig(endpoint string) string {
	return fmt.Sprintf(`
provider "linear" {
  token    = "lin_api_acctest"
  endpoint = %q
}
`, endpoint)
}
