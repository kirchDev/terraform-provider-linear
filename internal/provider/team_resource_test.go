package provider

import (
	"fmt"
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

// Optional + Computed buys "an attribute the configuration leaves out keeps its
// live value", and it is paid for with the only way HCL had to say *unset this*:
// `x = null` is indistinguishable from omitting `x`, so removing the attribute
// no longer clears anything. Free text still has an escape — `description = ""`
// is an empty description — but a UUID has no such value, so a nested team could
// not be un-nested through the provider at all.
//
// `""` on a reference is therefore read as the one intent it can carry and sent
// to Linear as an explicit null. The round trip is the half that has to hold:
// Linear answers a cleared reference with no reference at all, so state has to
// keep the `""` the plan promised. Map it back to null and the apply ends in
// "provider produced inconsistent result after apply" — and if it somehow got
// past that, the next refresh would rewrite state to null and reopen the same
// diff on every plan forever.
//
// Both decode shapes are covered here on purpose: default_issue_state_id and
// parent_id come back as a nested `{ id }` relation, auto_close_state_id as a
// bare scalar, and they are mapped by different helpers.
func TestAccTeam_emptyStringClearsAReference(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const (
		parent     = "b19c4ae2-0000-4000-8000-000000000003"
		issueState = "fc6401bf-0000-4000-8000-000000000001"
		closeState = "a5fc2074-0000-4000-8000-000000000002"
	)

	const nested = `
resource "linear_team" "test" {
  name                   = "Engineering"
  key                    = "ENG"
  parent_id              = "` + parent + `"
  default_issue_state_id = "` + issueState + `"
  auto_close_state_id    = "` + closeState + `"
}
`

	// The same team asking for all three references to be unset — the only way a
	// configuration can say so once the attributes are Optional + Computed.
	const cleared = `
resource "linear_team" "test" {
  name                   = "Engineering"
  key                    = "ENG"
  parent_id              = ""
  default_issue_state_id = ""
  auto_close_state_id    = ""
}
`

	isEmpty := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("linear_team.test", "parent_id", ""),
		resource.TestCheckResourceAttr("linear_team.test", "default_issue_state_id", ""),
		resource.TestCheckResourceAttr("linear_team.test", "auto_close_state_id", ""),
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + nested,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_team.test", "parent_id", parent),
					resource.TestCheckResourceAttr("linear_team.test", "default_issue_state_id", issueState),
					resource.TestCheckResourceAttr("linear_team.test", "auto_close_state_id", closeState),
				),
			},
			{
				Config: providerConfig(srv.URL) + cleared,
				Check: resource.ComposeAggregateTestCheckFunc(
					isEmpty,
					// State reading `""` is not on its own proof that Linear was told
					// anything: the mutation has to have carried a null, which is what
					// actually drops the relation. The mock stores what it was sent, so
					// the absence of the keys is that proof.
					func(*terraform.State) error {
						stored := mock.only(t, "team")
						for _, field := range []string{
							"parent", "parentId",
							"defaultIssueState", "defaultIssueStateId",
							"autoCloseState", "autoCloseStateId",
						} {
							if got, ok := stored[field]; ok {
								t.Errorf("team still holds %s = %#v: the update sent an empty string "+
									"rather than an explicit null", field, got)
							}
						}
						return nil
					},
				),
			},
			// And the clear settles rather than re-proposing itself: `""` in the
			// configuration and `""` in state are the same value.
			{
				Config: providerConfig(srv.URL) + cleared,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: isEmpty,
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

// Linear refuses a teamUpdate that carries the team's own unchanged
// securitySettings — `invalid input: team owners not available` on any workspace
// whose plan has no team owners, and the live value there is the empty object,
// so it is the attribute that is gated and not its content. An update echoing
// the whole team back therefore failed whatever the configuration changed, which
// made every in-place change to a team impossible (#42).
//
// Every readable attribute being Optional + Computed is what fills the plan with
// live values to echo, so the shared Update has to send only what actually
// moved. The assertion is on the raw input rather than the stored team: store
// merges the input onto what was already there, so a field that was seeded reads
// the same whether the provider sent it or not, and "did not send" is the whole
// claim.
func TestAccTeam_updateSendsOnlyWhatChanged(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const config = `
resource "linear_team" "test" {
  name = "Engineering"
  key  = "ENG"
}
`
	const described = `
resource "linear_team" "test" {
  name        = "Engineering"
  key         = "ENG"
  description = "Ships the product"
}
`

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
			// The refresh pulls Linear's defaults into state, so from here every
			// attribute the configuration never mentioned carries a live value into
			// the plan — the material the update used to echo back.
			{Config: providerConfig(srv.URL) + config},
			{
				Config: providerConfig(srv.URL) + described,
				Check: func(*terraform.State) error {
					inputs := mock.updateInputs("team")
					if len(inputs) == 0 {
						return fmt.Errorf("no teamUpdate was sent at all")
					}
					in := inputs[len(inputs)-1]
					if _, sent := in["securitySettings"]; sent {
						return fmt.Errorf("update echoed securitySettings, which Linear rejects: %#v", in)
					}
					if got := in["description"]; got != "Ships the product" {
						return fmt.Errorf("update did not carry the changed description, got %#v", got)
					}
					// The point is not merely that securitySettings is gone: an update
					// still echoing thirty other unchanged settings would pass the check
					// above and remain the same bug one field further on.
					if len(in) != 1 {
						return fmt.Errorf("update sent %d fields, want only the changed one: %#v", len(in), in)
					}
					return nil
				},
			},
		},
	})
}

// The diff can come out empty, and an empty update is still a request Linear can
// refuse. Dropping a write-only attribute from the configuration is the plainest
// way there: it is plain Optional rather than Optional + Computed, so Terraform
// plans `true -> null` and calls Update, while the provider has nothing to send —
// putBool skips a null it is not allowed to clear, and every other attribute is
// unchanged. What is left is a mutation carrying no fields at all, which is the
// echo bug's failure mode with none of its intent.
//
// So nothing left means no mutation. The refresh is the other half: state has to
// end up holding what is live even though nothing was written.
func TestAccTeam_updateWithNothingToSendSkipsTheMutation(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const shared = `
resource "linear_team" "test" {
  name                  = "Engineering"
  key                   = "ENG"
  issue_sharing_enabled = true
}
`
	const dropped = `
resource "linear_team" "test" {
  name = "Engineering"
  key  = "ENG"
}
`

	var before int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + shared,
				Check: func(*terraform.State) error {
					mock.fill(t, "team", mock.only(t, "team")["id"].(string), linearTeamDefaults)
					return nil
				},
			},
			{
				Config: providerConfig(srv.URL) + shared,
				Check: func(*terraform.State) error {
					before = len(mock.updateInputs("team"))
					return nil
				},
			},
			{
				Config: providerConfig(srv.URL) + dropped,
				Check: resource.ComposeAggregateTestCheckFunc(
					func(*terraform.State) error {
						if got := len(mock.updateInputs("team")); got != before {
							return fmt.Errorf("sent %d mutation(s) with nothing to change: %#v",
								got-before, mock.updateInputs("team")[before:])
						}
						return nil
					},
					// Skipping the mutation must not skip the refresh.
					resource.TestCheckResourceAttr("linear_team.test", "all_members_can_join", "true"),
					resource.TestCheckResourceAttr("linear_team.test", "security_settings_json", "{}"),
					resource.TestCheckResourceAttr("linear_team.test", "timezone", "Europe/Berlin"),
				),
			},
		},
	})
}

// The follow-up update after a create is the same echo one step earlier. Team's
// create input is narrower than its update input, so a configuration setting any
// update-only attribute gets a second mutation — and that one was built from the
// whole plan too, re-sending everything the create had just written. There is no
// prior state to compare against there, but there is a create response, and that
// is what the entity looks like at the moment the follow-up runs.
//
// So the follow-up sends the update-only attribute and nothing else: the create
// already landed the rest.
func TestAccTeam_createFollowUpUpdateSendsOnlyWhatChanged(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	// all_members_can_join is on TeamUpdateInput alone, so this is a create that
	// cannot land in one mutation.
	const config = `
resource "linear_team" "test" {
  name                 = "Engineering"
  key                  = "ENG"
  description          = "Ships the product"
  all_members_can_join = false
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: func(*terraform.State) error {
					inputs := mock.updateInputs("team")
					if len(inputs) != 1 {
						return fmt.Errorf("want exactly one follow-up teamUpdate, got %d: %#v", len(inputs), inputs)
					}
					in := inputs[0]
					if got, ok := in["allMembersCanJoin"]; !ok || got != false {
						return fmt.Errorf("follow-up update did not carry allMembersCanJoin, got %#v", in)
					}
					if len(in) != 1 {
						return fmt.Errorf("follow-up update re-sent %d fields the create already wrote: %#v",
							len(in)-1, in)
					}
					return nil
				},
			},
		},
	})
}

// needsUpdateAfterCreate answers off IsNull, and an Optional + Computed
// attribute the configuration never mentions is unknown at create rather than
// null — so every team create asks for a follow-up update, whether or not the
// configuration set anything the create input could not carry. Diffed against
// the create response that mutation has nothing left in it, and an empty
// teamUpdate is a request Linear can still refuse for reasons that have nothing
// to do with this apply.
func TestAccTeam_createSendsNoFollowUpUpdateWithNothingToChange(t *testing.T) {
	mock := newLinearMock()
	mock.expose("team", "Team", teamTypeFields)
	srv := mock.server(t)

	const config = `
resource "linear_team" "test" {
  name        = "Engineering"
  key         = "ENG"
  description = "Ships the product"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					func(*terraform.State) error {
						if inputs := mock.updateInputs("team"); len(inputs) != 0 {
							return fmt.Errorf("create sent %d follow-up teamUpdate(s) with nothing to change: %#v",
								len(inputs), inputs)
						}
						return nil
					},
					resource.TestCheckResourceAttr("linear_team.test", "description", "Ships the product"),
					resource.TestCheckResourceAttr("linear_team.test", "key", "ENG"),
				),
			},
		},
	})
}
