package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
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

	// exposes maps entity name → the type it selects on, for the entities whose
	// selection set should be validated. See expose.
	exposes map[string]exposedType
}

// exposedType is one Linear type's readable surface.
type exposedType struct {
	// typeName is the GraphQL type, e.g. "Team" — what the error message names.
	typeName string
	// fields is every field the type exposes.
	fields map[string]bool
	// composite maps each field whose own type is not a scalar to that type,
	// e.g. "ipRestrictions" → "[OrganizationIpRestriction!]". Selecting one of
	// these bare, as if it were a scalar, is the second way a selection set can
	// be invalid.
	composite map[string]string
}

// expose registers the fields a Linear type actually has, so every selection of
// `entity` in a document is checked against them.
//
// Without it the mock is an echo: a mutation stores whatever its input carried
// and a read hands the same map straight back, so a selection set can ask for a
// field the real type has never had and still round-trip green. The real
// endpoint validates the selection set against the schema and rejects the whole
// query — `Cannot query field "x" on type "Y".` — which is why a provider bug of
// exactly that shape can reach a release with every test passing.
//
// fields is whitespace-separated, in the same shape a resource writes its
// selection set. A field written `name{Type}` is one whose type is not a scalar:
// GraphQL rejects a bare selection of it just as firmly as it rejects a field
// that does not exist, and with a different message, so the two are registered
// and reported apart. An entity nobody registers is not validated, so this
// changes nothing for the tests that do not opt in.
func (m *linearMock) expose(entity, typeName, fields string) {
	exposed := exposedType{
		typeName:  typeName,
		fields:    map[string]bool{},
		composite: map[string]string{},
	}
	for _, f := range strings.Fields(fields) {
		name, fieldType, isComposite := strings.Cut(f, "{")
		exposed.fields[name] = true
		if isComposite {
			exposed.composite[name] = strings.TrimSuffix(fieldType, "}")
		}
	}
	if m.exposes == nil {
		m.exposes = map[string]exposedType{}
	}
	m.exposes[entity] = exposed
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

	if message, ok := m.invalidSelection(req.Query); ok {
		// Deliberately not an EntityNotFoundError: a query the schema rejects has
		// to reach the practitioner as a failure, not be mistaken for a deleted
		// resource and silently dropped from state.
		m.writeError(w, "GRAPHQL_VALIDATION_FAILED", message)
		return
	}

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

// invalidSelection reports the GraphQL validation error the real endpoint would
// answer a document with, for every entity expose registered — the first field
// the type it selects on does not have, or the first composite field selected
// without a selection set of its own. The message is the one Linear sends,
// because that is what a practitioner reads out of the failed plan.
func (m *linearMock) invalidSelection(doc string) (message string, found bool) {
	// Drop the operation definition, so an operation named after its entity —
	// `query team($id: String!)` — is not itself read as a selection of it.
	body := doc
	if i := strings.Index(body, "{"); i >= 0 {
		body = body[i:]
	}

	entities := make([]string, 0, len(m.exposes))
	for entity := range m.exposes {
		entities = append(entities, entity)
	}
	// Sorted so a document with several invalid fields always names the same one.
	sort.Strings(entities)

	for _, entity := range entities {
		exposed := m.exposes[entity]
		for _, block := range selectionBlocks(body, entity) {
			for _, f := range selectedFields(block) {
				switch {
				case !exposed.fields[f.name]:
					return fmt.Sprintf("Cannot query field %q on type %q.", f.name, exposed.typeName), true
				case exposed.composite[f.name] != "" && !f.selection:
					return fmt.Sprintf(
						"Field %q of type %q must have a selection of subfields. Did you mean %q { ... }?",
						f.name, exposed.composite[f.name], f.name), true
				}
			}
		}
	}
	return "", false
}

// selectionBlocks returns the body of every selection set the field `name`
// introduces — `team { … }` and `team(id: $id) { … }` alike. A read selects the
// entity directly; a mutation selects it inside its payload, so both shapes turn
// up in the documents this provider sends.
func selectionBlocks(doc, name string) []string {
	var blocks []string
	for i := 0; i < len(doc); {
		j := strings.Index(doc[i:], name)
		if j < 0 {
			break
		}
		start := i + j
		i = start + len(name)
		if !wholeToken(doc, start, len(name)) {
			continue
		}
		k := skipSpace(doc, i)
		if k < len(doc) && doc[k] == '(' {
			k = skipSpace(doc, skipBalanced(doc, k, '(', ')'))
		}
		if k >= len(doc) || doc[k] != '{' {
			continue
		}
		end := skipBalanced(doc, k, '{', '}')
		blocks = append(blocks, doc[k+1:end-1])
		i = end
	}
	return blocks
}

// selectedField is one field a selection block asks for.
type selectedField struct {
	name string
	// selection reports whether the field brought a selection set of its own —
	// `x { … }` rather than a bare `x`. A composite field must; a scalar cannot.
	selection bool
}

// selectedFields returns the fields a selection block asks for at its own level.
// A nested selection contributes its own name only — `parent { id }` selects
// `parent` on this type and `id` on another one, which this mock does not model.
func selectedFields(block string) []selectedField {
	var fields []selectedField
	for i := 0; i < len(block); {
		switch c := block[i]; {
		case c == '{':
			i = skipBalanced(block, i, '{', '}')
		case c == '(':
			i = skipBalanced(block, i, '(', ')')
		case identStart(c):
			j := i + 1
			for j < len(block) && identPart(block[j]) {
				j++
			}
			// Arguments sit between the name and its selection set —
			// `labels(first: 50) { … }` — so step over them before looking.
			k := skipSpace(block, j)
			if k < len(block) && block[k] == '(' {
				k = skipSpace(block, skipBalanced(block, k, '(', ')'))
			}
			fields = append(fields, selectedField{
				name:      block[i:j],
				selection: k < len(block) && block[k] == '{',
			})
			i = j
		default:
			i++
		}
	}
	return fields
}

func identStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func identPart(c byte) bool { return identStart(c) || (c >= '0' && c <= '9') }

// wholeToken reports whether the run of n bytes at start is a complete
// identifier, so looking for `team` never matches inside `teamCreate`.
func wholeToken(s string, start, n int) bool {
	if start > 0 && identPart(s[start-1]) {
		return false
	}
	end := start + n
	return end >= len(s) || !identPart(s[end])
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == ',') {
		i++
	}
	return i
}

// skipBalanced returns the index just past the delimiter closing the one at i.
func skipBalanced(s string, i int, opener, closer byte) int {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}

// only returns the single stored entity of a kind. A write-only attribute has
// no read-back, so what the mutation actually sent is the only thing a test can
// assert on — and that is what the mock stored.
func (m *linearMock) only(t *testing.T, name string) map[string]any {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if got := len(m.entities[name]); got != 1 {
		t.Fatalf("want exactly one stored %s, got %d", name, got)
	}
	for _, stored := range m.entities[name] {
		return stored
	}
	return nil
}

// seedSingleton pre-stores the one entity of a kind that exists without anyone
// having created it. The workspace is Linear's: there is no organizationCreate,
// `query organization` takes no id, and organizationUpdate updates whatever the
// API key points at. Entities are keyed by the id in the variables, so a
// document that sends none looks up the empty key — which is where this puts it.
func (m *linearMock) seedSingleton(name string, fields map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored := map[string]any{}
	for k, v := range fields {
		stored[k] = v
	}
	if m.entities[name] == nil {
		m.entities[name] = map[string]map[string]any{}
	}
	m.entities[name][""] = stored
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
	// A singleton is stored under the empty key, because the documents that read
	// and update it carry no id — but it still has one of its own, and that must
	// survive an update rather than be overwritten with the key.
	if id != "" {
		stored["id"] = id
	}

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
