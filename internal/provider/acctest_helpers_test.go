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

// queryTypeFields is what `type Query` exposes, copied from Linear's own SDL —
// `bash scripts/fetch-schema.sh`, then:
//
//	awk '/^type Query \{/,/^\}/' .linear-schema.graphql |
//	  grep -E '^  [a-zA-Z]+(\(|:)' | sed 's/(.*//;s/:.*//'
//
// It lives here rather than beside one test because it belongs to no entity: it
// is the list of doors into the API, and any resource's read may be knocking on
// one that is not there. Pass it to exposeRoot("query", "Query", …).
//
// Read it for what is absent as much as for what is present — there is a `team`
// and a `teams`, and nothing for a git automation state at all.
const queryTypeFields = `
	administrableTeams agentActivities agentActivity agentSession agentSessionSandbox
	agentSessions agentSkill agentSkills applicationInfo archivedIntegrations
	archivedTeams attachment attachmentIssue attachmentSources attachments
	attachmentsForURL auditEntries auditEntryTypes authenticationSessions availableUsers
	comment comments customView customViewDetailsSuggestion customViewHasSubscribers
	customViews customer customerNeed customerNeeds customerStatus customerStatuses
	customerTier customerTiers customers cycle cycles diff document
	documentContentHistory documentContentHistoryEntries documentContentHistoryTimeline
	documents emailIntakeAddress emoji emojis entityExternalLink externalUser
	externalUsers failuresForOauthWebhooks favorite favorites fetchData initiative
	initiativeFilterSuggestion initiativeLabel initiativeLabels
	initiativeLeadTeamChangeImpact initiativeRelation initiativeRelations
	initiativeToProject initiativeToProjects initiativeUpdate initiativeUpdates
	initiatives integration integrationHasScopes integrationTemplate integrationTemplates
	integrations integrationsSettings issue issueFigmaFileKeySearch issueFilterSuggestion
	issueImportCheckCSV issueImportCheckSync issueImportJqlCheck issueLabel issueLabels
	issuePriorityValues issueRelation issueRelations issueRepositorySuggestions
	issueSearch issueTitleSuggestionFromCustomerRequest issueToRelease issueToReleases
	issueVcsBranchSearch issues latestReleaseByAccessKey microsoftTeamsChannels
	notification notificationSubscription notificationSubscriptions notifications
	notificationsUnreadCount oauthApplication oauthApplications organization
	organizationDomainClaimRequest organizationExists organizationInvite
	organizationInviteDetails organizationInvites organizationMeta partnerOfferDetails
	partnerOfferWorkspaces project projectFilterSuggestion projectLabel projectLabels
	projectMilestone projectMilestones projectRelation projectRelations projectStatus
	projectStatusProjectCount projectStatuses projectUpdate projectUpdates projects
	pushSubscriptionTest rateLimitStatus recentReleasesByAccessKey release releaseNote
	releaseNotes releasePipeline releasePipelineByAccessKey releasePipelines
	releaseSearch releaseStage releaseStages releases roadmap roadmapToProject
	roadmapToProjects roadmaps searchDocuments searchIssues searchProjects semanticSearch
	slaConfigurations ssoUrlFromEmail team teamMembership teamMemberships teams template
	templates templatesForIntegration timeSchedule timeSchedules triageResponsibilities
	triageResponsibility user userSessions userSettings users
	verifyGitHubEnterpriseServerInstallation viewer webhook webhooks workflowState
	workflowStates
`

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
	// roots maps operation keyword ("query" / "mutation") → the root type its
	// outermost selection is validated against. See exposeRoot.
	roots map[string]exposedType

	// updates maps entity name → the inputs its update mutation was called with,
	// in order. The stored entity cannot answer what a mutation *sent*: store
	// merges the input onto what was already there, so a field that was seeded
	// looks identical whether the provider sent it or not. Asserting that an
	// attribute is NOT sent needs the raw input, which is what this keeps.
	updates map[string][]map[string]any
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
	if m.exposes == nil {
		m.exposes = map[string]exposedType{}
	}
	m.exposes[entity] = newExposedType(typeName, fields)
}

// exposeRoot registers the fields Query or Mutation itself has, so the field a
// document *enters* through is checked as well.
//
// expose answers "does this type have the fields the provider selects on it";
// this answers "is there a field here to select on at all". They are the two
// halves of one question, and the second half is the one that catches an entity
// read through a path the schema does not have — a selection set can be perfect
// and still be hung off a root field that was never there, which no amount of
// checking the type it selects on will notice.
//
// operation is the keyword the document opens with, "query" or "mutation".
func (m *linearMock) exposeRoot(operation, typeName, fields string) {
	if m.roots == nil {
		m.roots = map[string]exposedType{}
	}
	m.roots[operation] = newExposedType(typeName, fields)
}

// newExposedType parses one type's field list, in the whitespace-separated shape
// expose and exposeRoot both take.
func newExposedType(typeName, fields string) exposedType {
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
	return exposed
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
// answer a document with, for every type expose and exposeRoot registered — the
// first field the type it selects on does not have, or the first composite field
// selected without a selection set of its own. The message is the one Linear
// sends, because that is what a practitioner reads out of the failed plan.
func (m *linearMock) invalidSelection(doc string) (message string, found bool) {
	// The operation definition is dropped with the outer braces, so an operation
	// named after its entity — `query team($id: String!)` — is not itself read as
	// a selection of it.
	root, ok := rootBlock(doc)
	if !ok {
		return "", false
	}

	// The root field is a field of Query or Mutation, which are types like any
	// other: `{ gitAutomationState(id: $id) { … } }` is invalid the moment Query
	// has no such field, however well-formed everything inside it is.
	if exposed, registered := m.roots[documentOperation(doc)]; registered {
		if message, found := invalidFields(root, exposed); found {
			return message, true
		}
	}

	entities := make([]string, 0, len(m.exposes))
	for entity := range m.exposes {
		entities = append(entities, entity)
	}
	// Sorted so a document with several invalid fields always names the same one.
	sort.Strings(entities)

	for _, entity := range entities {
		exposed := m.exposes[entity]
		for _, block := range selectionBlocks(root, entity) {
			if message, found := invalidFields(block, exposed); found {
				return message, true
			}
		}
	}
	return "", false
}

// invalidFields reports the first field of a selection block that the type it is
// asked of would reject.
func invalidFields(block string, exposed exposedType) (message string, found bool) {
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
	return "", false
}

// rootBlock returns the contents of a document's outermost selection set — what
// it asks of Query or Mutation itself.
func rootBlock(doc string) (string, bool) {
	i := strings.Index(doc, "{")
	if i < 0 {
		return "", false
	}
	end := skipBalanced(doc, i, '{', '}')
	if end <= i+1 {
		return "", false
	}
	return doc[i+1 : end-1], true
}

// documentOperation returns the keyword a document opens with — "query" or
// "mutation" — which is the root type its outermost selection belongs to.
func documentOperation(doc string) string {
	fields := strings.Fields(doc)
	if len(fields) == 0 {
		return ""
	}
	switch fields[0] {
	case "query", "mutation":
		return fields[0]
	}
	return ""
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

// updateInputs returns every input the entity's update mutation was called
// with, in order. Use it to assert what a mutation did NOT send — `only` reads
// the merged result and cannot tell an unsent field from an unchanged one.
func (m *linearMock) updateInputs(name string) []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updates[name]
}

// seedSingleton pre-stores the one entity of a kind that exists without anyone
// having created it. The workspace is Linear's: there is no organizationCreate,
// `query organization` takes no id, and organizationUpdate updates whatever the
// API key points at. Entities are keyed by the id in the variables, so a
// document that sends none looks up the empty key — which is where this puts it.
func (m *linearMock) seedSingleton(name string, fields map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.put(name, "", fields)
}

// seed pre-stores an entity a configuration refers to but never creates — the
// team a git automation rule hangs off, say, which the configuration names by
// UUID. An entity reachable only through its parent needs that parent to be
// there, so a test that reads one has to stand it up.
func (m *linearMock) seed(name, id string, fields map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.put(name, id, fields)
}

// fill merges fields into an entity that already exists, standing in for the
// defaults Linear applies itself to everything a mutation did not send.
//
// The mock is otherwise an echo — a read returns what the create input carried
// — which quietly makes every unsent field come back as its zero value. That is
// not what the API does, and the difference is load-bearing for anything about
// optional-and-computed attributes: what those plan as depends on there being a
// real value in state to keep.
func (m *linearMock) fill(t *testing.T, name, id string, fields map[string]any) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := m.entities[name][id]
	if stored == nil {
		t.Fatalf("no stored %s %q to fill", name, id)
	}
	for k, v := range fields {
		stored[k] = v
	}
}

func (m *linearMock) put(name, id string, fields map[string]any) {
	stored := map[string]any{}
	for k, v := range fields {
		stored[k] = v
	}
	if id != "" {
		stored["id"] = id
	}
	if m.entities[name] == nil {
		m.entities[name] = map[string]map[string]any{}
	}
	m.entities[name][id] = stored
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
	if m.updates == nil {
		m.updates = map[string][]map[string]any{}
	}
	m.updates[name] = append(m.updates[name], input)

	stored := m.store(name, id, input, existing)
	m.writeData(w, map[string]any{
		name + "Update": map[string]any{"success": true, name: stored},
	})
}

func (m *linearMock) read(w http.ResponseWriter, name string, req mockRequest) {
	if m.readConnection(w, req) {
		return
	}

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

// readConnection answers a collection read out of the stored entities, in the
// two shapes this provider sends: `xs { nodes { … } }` asked of Query directly,
// and `parent(id:) { xs { nodes { … } } }`, where the collection hangs off the
// parent that scopes it. Linear reaches some entities only that second way — a
// git automation rule has no root query of its own — so a mock that answers
// x(id:) and nothing else cannot exercise their read path at all.
//
// The parent itself is not looked up: what scopes the children is the reference
// each of them stores, and a test that never created the parent is not thereby
// saying it does not exist.
//
// Everything comes back in one page. Pagination is the client's contract with
// the API, and what this checks is that the client honours `hasNextPage` rather
// than assuming a page it did not read is empty.
//
// It reports false for a document that is not connection-shaped, leaving the
// id-based read to answer it.
func (m *linearMock) readConnection(w http.ResponseWriter, req mockRequest) bool {
	root, ok := rootBlock(req.Query)
	if !ok {
		return false
	}
	rootFields := selectedFields(root)
	if len(rootFields) != 1 {
		return false
	}

	// The root field's own block is the first one it introduces: a nested
	// selection of the same name — `team { id }` inside the nodes — comes later.
	field := rootFields[0].name
	blocks := selectionBlocks(root, field)
	if len(blocks) == 0 {
		return false
	}
	block := blocks[0]

	if selects(block, "nodes") {
		m.writeData(w, map[string]any{field: m.page(singularOf(field), "", "")})
		return true
	}

	nested := selectedFields(block)
	if len(nested) != 1 {
		return false
	}
	child := nested[0].name
	childBlocks := selectionBlocks(block, child)
	if len(childBlocks) == 0 || !selects(childBlocks[0], "nodes") {
		return false
	}

	parentID, _ := req.Variables["id"].(string)
	m.writeData(w, map[string]any{
		field: map[string]any{child: m.page(singularOf(child), field, parentID)},
	})
	return true
}

// page returns one connection page of the stored entities of a kind — all of
// them, or, when parentField is set, the ones whose reference to that parent is
// the id being asked for.
func (m *linearMock) page(name, parentField, parentID string) map[string]any {
	ids := make([]string, 0, len(m.entities[name]))
	for id := range m.entities[name] {
		ids = append(ids, id)
	}
	// Sorted so a connection's order is stable rather than a map's.
	sort.Strings(ids)

	nodes := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		stored := m.entities[name][id]
		if parentField != "" && storedRefID(stored[parentField]) != parentID {
			continue
		}
		nodes = append(nodes, stored)
	}
	return map[string]any{
		"nodes":    nodes,
		"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
	}
}

// storedRefID reads the id out of a stored relation — `team: {id: …}`, the shape
// store translates a `teamId` input into.
func storedRefID(v any) string {
	ref, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	id, _ := ref["id"].(string)
	return id
}

// singularOf turns a collection field into the name the mock stores its entities
// under — `teams` → `team`, `gitAutomationStates` → `gitAutomationState`.
func singularOf(field string) string { return strings.TrimSuffix(field, "s") }

// selects reports whether a selection block asks for a field at its own level.
func selects(block, name string) bool {
	for _, f := range selectedFields(block) {
		if f.name == name {
			return true
		}
	}
	return false
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

// forget drops every stored entity of a kind, standing in for them being deleted
// in Linear behind Terraform's back.
func (m *linearMock) forget(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entities, name)
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
