package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// teamTypeFields is what `type Team` exposes, copied from Linear's own SDL —
// `bash scripts/fetch-schema.sh`, then:
//
//	awk '/^type Team implements Node \{/,/^\}/' .linear-schema.graphql |
//	  grep -E '^  [a-zA-Z]+(\(|:)' | sed 's/(.*//;s/:.*//'
//
// It is a snapshot of the API's contract, not a restatement of what the provider
// selects, which is the whole point: the two can disagree, and when they do this
// is what says so. The schema file itself is ~1.2 MB and gitignored, so the list
// is committed here rather than read at test time.
const teamTypeFields = `
	activeCycle aiDiscussionSummariesEnabled aiThreadSummariesEnabled allMembersCanJoin
	ancestors archivedAt autoArchivePeriod autoCloseChildIssues autoCloseParentIssues
	autoClosePeriod autoCloseStateId children color createdAt currentProgress
	cycleCalenderUrl cycleCooldownTime cycleDuration cycleIssueAutoAssignCompleted
	cycleIssueAutoAssignStarted cycleLockToActive cycleStartDay cycles cyclesEnabled
	defaultIssueEstimate defaultIssueState defaultProjectTemplate defaultTemplateForMembers
	defaultTemplateForMembersId defaultTemplateForNonMembers defaultTemplateForNonMembersId
	description displayName draftWorkflowState facets gitAutomationStates groupIssueHistory
	icon id inheritIssueEstimation inheritSlackAutoCreateProjectChannel
	inheritWorkflowStatuses initiativesEnabled integrationsSettings inviteHash issueCount
	issueEstimationAllowZero issueEstimationExtended issueEstimationType
	issueOrderingNoPriorityFirst issueSortOrderDefaultToBottom issues joinByDefault key
	labels ledInitiativeCount markedAsDuplicateWorkflowState members membership memberships
	mergeWorkflowState mergeableWorkflowState name organization parent pinnedResources posts
	private progressHistory projects protectedBy protectedById releasePipelines
	requirePriorityToLeaveTriage resourceSections restrictedBy restrictedById retiredAt
	reviewWorkflowState scimGroupName scimManaged securitySettings
	setIssueSortOrderOnStateChange slackAutoCreateProjectChannel slackIssueComments
	slackIssueStatuses slackNewIssue startWorkflowState states templates timezone
	triageEnabled triageIssueState triageResponsibility upcomingCycleCount updatedAt
	visibility webhooks
`

// The team read selected issueSharingEnabled, which type Team does not have.
// Linear validates a selection set against the type and rejects the whole query,
// so this was not one bad attribute — it was every plan, refresh, import and
// apply touching any team, with no configuration that avoided it.
//
// The guard is deliberately wider than the one field: nothing the provider reads
// off a team may be absent from Team.
func TestAccTeam_selectsOnlyFieldsTeamExposes(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const config = `
resource "linear_team" "test" {
  name     = "Engineering"
  key      = "ENG"
  timezone = "Europe/Berlin"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_team.test", "name", "Engineering"),
					resource.TestCheckResourceAttr("linear_team.test", "key", "ENG"),
					resource.TestCheckResourceAttrSet("linear_team.test", "id"),
				),
			},
			{
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "linear_team.test",
				ImportState:       true,
				ImportStateVerify: true,
				Config:            providerConfig(srv.URL) + config,
			},
		},
	})
}

// linearTeamDefaults is a team as Linear reports one back: every setting the
// configuration never mentioned, answered with the API's own default rather
// than with nothing. The values are the ones from the report in #24.
var linearTeamDefaults = map[string]any{
	"private":  false,
	"timezone": "Europe/Berlin",

	"cyclesEnabled":                 false,
	"cycleStartDay":                 float64(1),
	"cycleIssueAutoAssignStarted":   true,
	"cycleIssueAutoAssignCompleted": true,
	"upcomingCycleCount":            float64(2),

	"triageEnabled":                false,
	"requirePriorityToLeaveTriage": false,

	"autoArchivePeriod":   float64(6),
	"issueEstimationType": "notUsed",

	"groupIssueHistory":              true,
	"setIssueSortOrderOnStateChange": "first",
	"initiativesEnabled":             false,

	"inheritIssueEstimation":  false,
	"inheritWorkflowStatuses": false,

	"aiThreadSummariesEnabled":     true,
	"aiDiscussionSummariesEnabled": true,

	"allMembersCanJoin": true,
	"joinByDefault":     false,

	"securitySettings": map[string]any{},
}

// Optional+Computed is how this provider says "Linear defaults this one
// itself, and leaving it out of the configuration means keep what is live".
// The plan used to contradict that: the framework marks a Computed attribute
// unknown from the *configuration*, so renaming a team turned every setting the
// configuration never mentioned into "(known after apply)" — thirty values that
// were known, in state, and about to be left exactly as they were.
//
// The rename is the whole trigger. A resource with nothing to change was
// already clean, which is why the case hid behind every ExpectEmptyPlan the
// suite had; it takes one real change to surface it, and then it surfaces on
// every other attribute at once.
func TestAccTeam_unsetOptionalComputedKeepsItsValue(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const config = `
resource "linear_team" "test" {
  name = "Engineering"
  key  = "ENG"
}
`
	const renamed = `
resource "linear_team" "test" {
  name = "Platform"
  key  = "ENG"
}
`

	keeps := func(attribute string, value knownvalue.Check) plancheck.PlanCheck {
		return plancheck.ExpectKnownValue("linear_team.test", tfjsonpath.New(attribute), value)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: func(*terraform.State) error {
					mock.fill(t, "team", mock.only(t, "team")["id"].(string), linearTeamDefaults)
					return nil
				},
			},
			// The refresh pulls Linear's defaults into state, so from here the
			// prior value of every unset attribute is known — the left-hand side
			// of the report's `true -> (known after apply)`.
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_team.test", "all_members_can_join", "true"),
					resource.TestCheckResourceAttr("linear_team.test", "ai_thread_summaries_enabled", "true"),
					resource.TestCheckResourceAttr("linear_team.test", "set_issue_sort_order_on_state_change", "first"),
					resource.TestCheckResourceAttr("linear_team.test", "security_settings_json", "{}"),
				),
			},
			{
				Config: providerConfig(srv.URL) + renamed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						keeps("name", knownvalue.StringExact("Platform")),
						keeps("all_members_can_join", knownvalue.Bool(true)),
						keeps("ai_thread_summaries_enabled", knownvalue.Bool(true)),
						keeps("ai_discussion_summaries_enabled", knownvalue.Bool(true)),
						keeps("cycle_issue_auto_assign_completed", knownvalue.Bool(true)),
						keeps("inherit_issue_estimation", knownvalue.Bool(false)),
						keeps("inherit_workflow_statuses", knownvalue.Bool(false)),
						keeps("initiatives_enabled", knownvalue.Bool(false)),
						keeps("require_priority_to_leave_triage", knownvalue.Bool(false)),
						keeps("set_issue_sort_order_on_state_change", knownvalue.StringExact("first")),
						keeps("timezone", knownvalue.StringExact("Europe/Berlin")),
						keeps("upcoming_cycle_count", knownvalue.Float64Exact(2)),
						keeps("security_settings_json", knownvalue.StringExact("{}")),
					},
				},
				Check: resource.TestCheckResourceAttr("linear_team.test", "name", "Platform"),
			},
			// And the rename settles: the plan after it is empty, so the noise
			// was never something an apply worked off.
			{
				Config: providerConfig(srv.URL) + renamed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// issue_sharing_enabled is settable and unreadable: TeamCreateInput and
// TeamUpdateInput both carry it, type Team does not return it. That makes it
// write-only in this provider's sense — the mutation still sends it, and state
// keeps what the config declared because there is nothing to refresh against.
//
// Both halves matter and each has its own failure. Drop it from the input and
// the attribute silently stops doing anything, with state still reporting the
// value the practitioner asked for. Leave it Computed and every apply ends in
// "provider produced inconsistent result", because the value Terraform planned
// is not one Linear ever sends back.
func TestAccTeam_issueSharingEnabledIsWriteOnly(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const config = `
resource "linear_team" "test" {
  name                  = "Engineering"
  key                   = "ENG"
  issue_sharing_enabled = true
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check:  resource.TestCheckResourceAttr("linear_team.test", "issue_sharing_enabled", "true"),
			},
			{
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: func(*terraform.State) error {
					if got := mock.only(t, "team")["issueSharingEnabled"]; got != true {
						t.Fatalf("mutation did not send issueSharingEnabled: got %#v", got)
					}
					return nil
				},
			},
		},
	})
}
