package provider

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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
