package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

// A team carries settings a human sets in the Linear UI — an icon, a default
// issue state, the state auto-closed issues move to. A configuration that never
// mentions them is not asking for them to be erased, so an apply must leave them
// exactly as they are.
//
// Plain Optional cannot do that. Terraform reads a null configuration value for
// a non-computed attribute as "make it null", so the refresh reads the live value
// into state, the plan proposes state → null, and the update sends an explicit
// null that wipes it. That is data loss on an attribute nobody wrote down, which
// is why linear_workspace_settings is Optional + Computed throughout and why
// linear_team has to be too.
func TestAccTeam_keepsLiveValuesTheConfigDoesNotMention(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const (
		issueState = "fc6401bf-0000-4000-8000-000000000001"
		closeState = "a5fc2074-0000-4000-8000-000000000002"
	)

	const managed = `
resource "linear_team" "test" {
  name                   = "Engineering"
  key                    = "ENG"
  description            = "Ships the product"
  icon                   = "Server"
  default_issue_state_id = "` + issueState + `"
  auto_close_state_id    = "` + closeState + `"
}
`

	// The same team, with every one of those attributes dropped from the
	// configuration — the shape a practitioner writes when the values are managed
	// in the UI rather than here.
	const unmanaged = `
resource "linear_team" "test" {
  name = "Engineering"
  key  = "ENG"
}
`

	live := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("linear_team.test", "description", "Ships the product"),
		resource.TestCheckResourceAttr("linear_team.test", "icon", "Server"),
		resource.TestCheckResourceAttr("linear_team.test", "default_issue_state_id", issueState),
		resource.TestCheckResourceAttr("linear_team.test", "auto_close_state_id", closeState),
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + managed,
				Check:  live,
			},
			{
				// Dropping an attribute from the configuration is not an instruction
				// to clear it: there is nothing left to change, so the plan is empty
				// and the live values survive.
				Config: providerConfig(srv.URL) + unmanaged,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: live,
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
