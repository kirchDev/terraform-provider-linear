package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	// `resource` in this file is the acceptance-testing helper, so the framework
	// package the Schema method takes its request and response from is aliased.
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// organizationTypeFields is what `type Organization` exposes, copied from
// Linear's own SDL — `bash scripts/fetch-schema.sh`, then:
//
//	awk '/^type Organization implements Node \{/,/^\}/' .linear-schema.graphql |
//	  grep -E '^  [a-zA-Z]+(\(|:)' | sed 's/(.*//;s/:.*//'
//
// A field written `name{Type}` is one whose own type is not a scalar, so a
// document has to select subfields of it; the paginated connections declare
// their type on the closing `):` line rather than beside the name.
//
// It is a snapshot of the API's contract, not a restatement of what the provider
// selects, which is the whole point: the two can disagree, and when they do this
// is what says so. The schema file itself is ~1.2 MB and gitignored, so the list
// is committed here rather than read at test time.
const organizationTypeFields = `
	agentAutomationEnabled aiAddonEnabled aiDiscussionSummariesEnabled aiProviderConfiguration
	aiTelemetryEnabled aiThreadSummariesEnabled allowMembersToInvite allowedAiProviders
	allowedAuthServices allowedFileUploadContentTypes archivedAt authSettings
	codeIntelligenceEnabled codeIntelligenceRepository codingAgentEnabled codingAgentSettings
	createdAt createdIssueCount customerCount customersConfiguration customersEnabled
	defaultFeedSummarySchedule defaultHomeView defaultHomeViewTargetId deletionRequestedAt
	facets{[Facet!]!} feedEnabled fiscalYearStartMonth generatedUpdatesEnabled gitBranchFormat
	gitLinkbackDescriptionsEnabled gitLinkbackMessagesEnabled gitPublicLinkbackMessagesEnabled
	hideNonPrimaryOrganizations hipaaComplianceEnabled id
	initiativeUpdateReminderFrequencyInWeeks initiativeUpdateRemindersDay
	initiativeUpdateRemindersHour integrations{IntegrationConnection!}
	ipRestrictions{[OrganizationIpRestriction!]} labels{IssueLabelConnection!}
	linearAgentEnabled linearAgentSettings logoUrl name periodUploadVolume previousUrlKeys
	projectLabels{ProjectLabelConnection!} projectStatuses{[ProjectStatus!]!}
	projectUpdateReminderFrequencyInWeeks projectUpdateRemindersDay projectUpdateRemindersHour
	projectUpdatesReminderFrequency pullRequestIssueMode pullRequestTourEnabled releaseChannel
	releasesEnabled restrictAgentInvocationToMembers restrictLabelManagementToAdmins
	restrictTeamCreationToAdmins roadmapEnabled samlEnabled samlSettings scimEnabled scimSettings
	securitySettings slaDayCount slackAutoCreateProjectChannel
	slackProjectChannelIntegration{Integration} slackProjectChannelPrefix
	slackProjectChannelsEnabled subscription{PaidSubscription} teams{TeamConnection!}
	templates{TemplateConnection!} themeSettings trialEndsAt trialStartsAt updatedAt urlKey
	userCount users{UserConnection!} workingDays
`

// organizationSecuritySettingsInputFields is every key `securitySettings`
// accepts, copied from Linear's own SDL — `bash scripts/fetch-schema.sh`, then:
//
//	awk '/^input OrganizationSecuritySettingsInput \{/,/^\}/' .linear-schema.graphql |
//	  grep -E '^  [a-zA-Z]+:' | sed 's/:.*//'
//
// Like organizationTypeFields above it is a snapshot of the API's contract
// rather than a restatement of what the provider sends — `securitySettings` is
// a JSONObject, so nothing in the provider or in Terraform ever checks a key
// written into it.
const organizationSecuritySettingsInputFields = `
	agentGuidanceRole apiSettingsRole automationManagementRole importRole integrationCreationRole
	invitationsRole labelManagementRole personalApiKeysRole teamCreationRole templateManagementRole
	workspaceInitiativesRole
`

// userRoleTypeValues is `enum UserRoleType`, from the same SDL. Every key of
// OrganizationSecuritySettingsInput takes one — the *minimum role* the setting
// requires — so the object holds no booleans at all. `user` and `admin` are the
// two a security setting is written with in practice.
const userRoleTypeValues = `admin app guest owner user`

// deprecatedOrganizationRestrictionFields are the top-level Organization fields
// the role ladder replaced. The SDL still carries them, each deprecated with
// "Use `securitySettings.<x>Role` instead", and this provider exposes none of
// them — so naming one in the documentation of security_settings_json points a
// practitioner at a key that object does not have.
var deprecatedOrganizationRestrictionFields = []string{
	"allowMembersToInvite",
	"restrictTeamCreationToAdmins",
	"restrictLabelManagementToAdmins",
}

// seedWorkspace stands the workspace singleton up in the mock. There is no
// organizationCreate to reach it through, so without this the very first
// organizationUpdate answers EntityNotFoundError.
func seedWorkspace(mock *linearMock) {
	mock.seedSingleton("organization", map[string]any{
		"id":     "00000000-0000-4000-8000-000000000001",
		"name":   "kirchDev",
		"urlKey": "kirchdev",
		// GraphQL answers with every subfield the document selected, so a
		// restriction with no description comes back carrying an explicit null.
		"ipRestrictions": []any{map[string]any{
			"description": nil,
			"enabled":     true,
			"range":       "203.0.113.0/24",
			"type":        "allow",
		}},
	})
}

// The workspace read selected ipRestrictions bare. It is a field type
// Organization has, but its type is [OrganizationIpRestriction!] rather than the
// JSONObject every other settings blob on the workspace is, so GraphQL rejects
// the document for want of a subfield selection — and rejects the whole
// document, not the one field. Both readers of the workspace build their
// selection set by hand, and the resource's is also the mutation's, so this was
// every plan, refresh, import and apply touching the workspace at once.
//
// The guard is deliberately wider than the one field: everything the provider
// reads off the workspace has to be a field Organization exposes, selected the
// way its own type requires.
func TestAccWorkspaceSettings_selectsOnlyFieldsOrganizationExposes(t *testing.T) {
	mock := newLinearMock()
	mock.expose("organization", "Organization", organizationTypeFields)
	seedWorkspace(mock)
	srv := mock.server(t)

	const config = `
resource "linear_workspace_settings" "test" {
  name                    = "kirchDev"
  fiscal_year_start_month = 0
}

data "linear_organization" "test" {}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_workspace_settings.test", "name", "kirchDev"),
					resource.TestCheckResourceAttrSet("linear_workspace_settings.test", "id"),
					resource.TestCheckResourceAttr("data.linear_organization.test", "url_key", "kirchdev"),
				),
			},
			{
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "linear_workspace_settings.test",
				ImportState:       true,
				ImportStateVerify: true,
				Config:            providerConfig(srv.URL) + config,
			},
		},
	})
}

// Linear refuses an organizationUpdate that carries the organization's own
// unchanged url key — `invalid input: non-unique organization url key` — so an
// update echoing the whole workspace back fails whatever the configuration
// changed. Every attribute here being Optional + Computed is what fills the
// plan with live values to echo, so the update has to send only what moved.
//
// The assertion is on the raw input rather than the stored entity: store merges
// the input onto what was seeded, so a field that was already there reads the
// same whether the provider sent it or not, and "did not send" is the whole
// claim.
func TestAccWorkspaceSettings_updateSendsOnlyWhatChanged(t *testing.T) {
	mock := newLinearMock()
	mock.expose("organization", "Organization", organizationTypeFields)
	seedWorkspace(mock)
	srv := mock.server(t)

	const adopt = `
resource "linear_workspace_settings" "test" {
  name = "kirchDev"
}
`
	const renamed = `
resource "linear_workspace_settings" "test" {
  name = "IT-Dienstleistungen Titus Kirch"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerConfig(srv.URL) + adopt},
			{
				Config: providerConfig(srv.URL) + renamed,
				Check: func(*terraform.State) error {
					inputs := mock.updateInputs("organization")
					if len(inputs) == 0 {
						return fmt.Errorf("no organizationUpdate was sent at all")
					}
					in := inputs[len(inputs)-1]
					if _, sent := in["urlKey"]; sent {
						return fmt.Errorf("update sent urlKey unchanged, which Linear rejects: %#v", in)
					}
					if got := in["name"]; got != "IT-Dienstleistungen Titus Kirch" {
						return fmt.Errorf("update did not carry the changed name, got %#v", got)
					}
					// The point is not merely that urlKey is gone: an update that
					// still echoed forty other unchanged settings would pass the
					// check above and remain the same bug one field further on.
					if len(in) != 1 {
						return fmt.Errorf("update sent %d fields, want only the changed one: %#v", len(in), in)
					}
					return nil
				},
			},
		},
	})
}

// ip_restrictions_json has to survive the round trip a practitioner actually
// writes: an entry with no description at all. Linear answers a selection with
// every subfield it was asked for, so that entry comes back carrying
// `"description": null` — which is the same restriction, and not the same JSON.
// Left as-is it is a value Terraform never planned, so the apply that wrote it
// ends in "provider produced inconsistent result after apply".
//
// decode is the seam: it is where the read shape is turned into the attribute,
// and the asymmetry is between what Linear sends and what the configuration
// said, not anything a later layer can still see.
func TestWorkspaceSettingsDecodeOmitsUnsetIPRestrictionDescription(t *testing.T) {
	// Exactly what `query organization { organization { … ipRestrictions { … } } }`
	// answers for the example the attribute's own documentation gives.
	const raw = `{
		"id": "00000000-0000-4000-8000-000000000001",
		"name": "kirchDev",
		"urlKey": "kirchdev",
		"ipRestrictions": [
			{"description": null, "enabled": true, "range": "203.0.113.0/24", "type": "allow"},
			{"description": "office", "enabled": false, "range": "198.51.100.0/24", "type": "deny"}
		]
	}`

	var m workspaceSettingsModel
	if err := m.decode(context.Background(), json.RawMessage(raw)); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// The configuration's own spelling: no description key where there is no
	// description, and one where there is.
	const want = `[
		{"range": "203.0.113.0/24", "type": "allow", "enabled": true},
		{"range": "198.51.100.0/24", "type": "deny", "enabled": false, "description": "office"}
	]`

	if !sameJSON(t, m.IPRestrictionsJSON.ValueString(), want) {
		t.Fatalf("ip_restrictions_json did not round-trip:\n got %s\nwant %s",
			m.IPRestrictionsJSON.ValueString(), want)
	}
}

// A workspace with no IP restrictions at all reads as null, not as an empty
// array: Linear declares the field nullable, and a configuration that never
// mentions the attribute must not start planning `[]` against it.
func TestWorkspaceSettingsDecodeNullIPRestrictions(t *testing.T) {
	var m workspaceSettingsModel
	if err := m.decode(context.Background(), json.RawMessage(`{"id":"x","ipRestrictions":null}`)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !m.IPRestrictionsJSON.IsNull() {
		t.Fatalf("want null ip_restrictions_json, got %q", m.IPRestrictionsJSON.ValueString())
	}
}

// security_settings_json carries raw JSON, so no layer between the practitioner
// and Linear checks the keys a configuration puts in it: the provider forwards
// the object, and Terraform only compares it. The attribute is also Optional +
// Computed and compared semantically, so a wrong key does not error either —
// the apply writes something Linear ignores and the plan never converges. The
// example and the attribute description are therefore the whole of what a
// practitioner has to go on, and tfplugindocs renders both into
// docs/resources/workspace_settings.md verbatim.
//
// They shipped naming allowMembersToInvite / restrictTeamCreationToAdmins /
// restrictLabelManagementToAdmins with boolean values. Those are the deprecated
// top-level Organization fields, not keys of securitySettings, and that object
// holds no booleans at all — every key of it takes a UserRoleType.
func TestWorkspaceSettingsExampleUsesRealSecuritySettingsKeys(t *testing.T) {
	path := filepath.Join(examplesDir(t), "resources", "linear_workspace_settings", "resource.tf")
	pairs := parseJSONEncodeBlock(t, path, "security_settings_json")
	if len(pairs) == 0 {
		t.Fatalf("%s: the security_settings_json example is empty, so it documents nothing", path)
	}

	accepted := nameSet(organizationSecuritySettingsInputFields)
	roles := nameSet(userRoleTypeValues)

	for _, pair := range pairs {
		if !accepted[pair.key] {
			t.Errorf("%s: security_settings_json sets %q, which OrganizationSecuritySettingsInput does not accept",
				path, pair.key)
		}
		role, opened := strings.CutPrefix(pair.value, `"`)
		role, closed := strings.CutSuffix(role, `"`)
		if !opened || !closed || !roles[role] {
			t.Errorf("%s: security_settings_json sets %s = %s; every key takes a UserRoleType (%s), never a boolean",
				path, pair.key, pair.value, strings.Join(strings.Fields(userRoleTypeValues), ", "))
		}
	}
}

// The description is the other half of the same page — the attribute reference
// tfplugindocs renders under ### Optional — and it carried the same three
// deprecated field names as the example did.
func TestWorkspaceSettingsSecuritySettingsDescriptionNamesRealKeys(t *testing.T) {
	var schemaResp frameworkresource.SchemaResponse
	NewWorkspaceSettingsResource().Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResp)

	attr, ok := schemaResp.Schema.Attributes["security_settings_json"]
	if !ok {
		t.Fatal("linear_workspace_settings has no security_settings_json attribute")
	}
	description := attr.GetMarkdownDescription()

	for _, field := range deprecatedOrganizationRestrictionFields {
		if strings.Contains(description, field) {
			t.Errorf("security_settings_json is described in terms of %s, a deprecated top-level "+
				"Organization field rather than a key of securitySettings", field)
		}
	}

	accepted := nameSet(organizationSecuritySettingsInputFields)
	for _, name := range strings.Split(description, "`") {
		if strings.HasSuffix(name, "Role") && !accepted[name] {
			t.Errorf("security_settings_json names the key %q, which OrganizationSecuritySettingsInput "+
				"does not accept", name)
		}
	}
}

// nameSet turns one of the whitespace-separated SDL snapshots above into a set.
func nameSet(names string) map[string]bool {
	set := map[string]bool{}
	for _, name := range strings.Fields(names) {
		set[name] = true
	}
	return set
}

type jsonEncodePair struct{ key, value string }

// parseJSONEncodeBlock reads the `key = value` pairs out of an
// `<attribute> = jsonencode({ … })` block in a Terraform example. Enough of a
// parser for the flat, one-pair-per-line objects the examples are written with,
// and deliberately no more: anything it cannot read is a failure rather than a
// silent skip, so an example it stops recognising cannot quietly go unchecked.
func parseJSONEncodeBlock(t *testing.T, path, attribute string) []jsonEncodePair {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	lines := strings.Split(string(src), "\n")
	body := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, attribute+" ") && strings.Contains(trimmed, "jsonencode({") {
			body = i + 1
			break
		}
	}
	if body < 0 {
		t.Fatalf("%s: no `%s = jsonencode({` block", path, attribute)
	}

	var pairs []jsonEncodePair
	for _, line := range lines[body:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "})") {
			return pairs
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			t.Fatalf("%s: cannot read %q as a `key = value` pair", path, trimmed)
		}
		pairs = append(pairs, jsonEncodePair{strings.TrimSpace(key), strings.TrimSpace(value)})
	}

	t.Fatalf("%s: the `%s = jsonencode({` block is never closed", path, attribute)
	return nil
}

// sameJSON compares two JSON documents the way the ip_restrictions_json
// attribute does — semantically, so key order and whitespace do not decide it.
func sameJSON(t *testing.T, got, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(got), &a); err != nil {
		t.Fatalf("parsing got: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("parsing want: %v", err)
	}
	return reflect.DeepEqual(a, b)
}
