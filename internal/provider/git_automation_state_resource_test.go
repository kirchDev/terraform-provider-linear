package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// gitAutomationStateTypeFields is what `type GitAutomationState` exposes, copied
// from Linear's own SDL — `bash scripts/fetch-schema.sh`, then:
//
//	awk '/^type GitAutomationState implements Node \{/,/^\}/' .linear-schema.graphql |
//	  grep -E '^  [a-zA-Z]+(\(|:)' | sed 's/(.*//;s/:.*//'
//
// branchPattern is deprecated in favour of targetBranch but still selectable, so
// it is listed: this is the type's surface, not a recommendation.
const gitAutomationStateTypeFields = `
	archivedAt branchPattern createdAt event id state{WorkflowState}
	targetBranch{GitAutomationTargetBranch} team{Team} updatedAt
`

// gitAutomationStatePayloadFields is what `type GitAutomationStatePayload`
// exposes — the wrapper every gitAutomationState mutation answers with, and the
// field the client reaches through to get at what it created or updated.
const gitAutomationStatePayloadFields = `gitAutomationState{GitAutomationState} lastSyncId success`

// The team every rule in this file hangs off. It is seeded rather than created:
// the configurations here name a team by UUID, and a rule is only reachable
// through the team that owns it.
const gitAutomationTeamID = "00000000-0000-4000-8000-000000000999"

// The `merge` event is the one the community provider cannot round-trip: it
// bundles all five git events into a single resource, so the read cannot tell an
// unset event from a deleted one and a declared `merge` shows up as a permanent
// diff. One resource per event is what fixes that, and this test pins it — all
// five events declared side by side, then an empty plan.
func TestAccGitAutomationState_allFiveEventsRoundTrip(t *testing.T) {
	mock := newLinearMock()
	mock.seed("team", gitAutomationTeamID, map[string]any{"key": "ENG", "name": "Engineering"})
	srv := mock.server(t)

	config := fmt.Sprintf(`
locals {
  team  = %q
  state = "00000000-0000-4000-8000-000000000888"
}`, gitAutomationTeamID) + `

resource "linear_git_automation_state" "draft" {
  team_id  = local.team
  event    = "draft"
  state_id = local.state
}

resource "linear_git_automation_state" "start" {
  team_id  = local.team
  event    = "start"
  state_id = local.state
}

resource "linear_git_automation_state" "review" {
  team_id  = local.team
  event    = "review"
  state_id = local.state
}

resource "linear_git_automation_state" "mergeable" {
  team_id  = local.team
  event    = "mergeable"
  state_id = local.state
}

resource "linear_git_automation_state" "merge" {
  team_id  = local.team
  event    = "merge"
  state_id = local.state
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_git_automation_state.merge", "event", "merge"),
					resource.TestCheckResourceAttr("linear_git_automation_state.mergeable", "event", "mergeable"),
					resource.TestCheckResourceAttr("linear_git_automation_state.draft", "event", "draft"),
				),
			},
			{
				// The assertion: `merge` in particular comes back as declared.
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "linear_git_automation_state.merge",
				ImportState:       true,
				ImportStateVerify: true,
				Config:            providerConfig(srv.URL) + config,
			},
		},
	})

	if got := mock.count("gitAutomationState"); got != 0 {
		t.Errorf("mock still holds %d automation states after destroy, want 0", got)
	}
}

// Linear's Query type has no gitAutomationState field, and no plural one either
// — the only way back to a rule is Team.gitAutomationStates. The read named a
// root gitAutomationState anyway, so every plan, refresh, import and apply
// touching a rule died at the API, with no configuration that avoided it.
//
// The selection set inside that read was correct throughout, which is why
// nothing caught it: what did not exist was the way in. So this checks the read
// from both ends — the root fields a document may enter through, and the fields
// Team and GitAutomationState really have once it is inside.
func TestAccGitAutomationState_readsThroughTheTeam(t *testing.T) {
	mock := newLinearMock()
	mock.exposeRoot("query", "Query", queryTypeFields)
	mock.expose("team", "Team", teamTypeFields)
	mock.expose("gitAutomationState", "GitAutomationState", gitAutomationStateTypeFields)
	// A plan never executes a create, so the write path went unexercised while the
	// read was failing. These pin the payloads the mutations answer with, which is
	// what the client reaches through for the entity it just wrote.
	mock.expose("gitAutomationStateCreate", "GitAutomationStatePayload", gitAutomationStatePayloadFields)
	mock.expose("gitAutomationStateUpdate", "GitAutomationStatePayload", gitAutomationStatePayloadFields)
	mock.expose("gitAutomationStateDelete", "DeletePayload", "entityId lastSyncId success")
	mock.seed("team", gitAutomationTeamID, map[string]any{"key": "ENG", "name": "Engineering"})
	srv := mock.server(t)

	rule := func(stateID string) string {
		return fmt.Sprintf(`
resource "linear_git_automation_state" "merge" {
  team_id  = %q
  event    = "merge"
  state_id = %q
}
`, gitAutomationTeamID, stateID)
	}
	const (
		done     = "00000000-0000-4000-8000-000000000888"
		released = "00000000-0000-4000-8000-000000000777"
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// The refresh the framework runs after the apply is the assertion:
				// it is the first read, and it is where the missing root field bit.
				Config: providerConfig(srv.URL) + rule(done),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_git_automation_state.merge", "event", "merge"),
					resource.TestCheckResourceAttr("linear_git_automation_state.merge", "team_id", gitAutomationTeamID),
					resource.TestCheckResourceAttr("linear_git_automation_state.merge", "state_id", done),
				),
			},
			{
				Config: providerConfig(srv.URL) + rule(done),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// An update in place, so the mutation is exercised rather than only
				// the create — a plan executes neither, which is how the write path
				// stayed unexamined while the read was failing.
				Config: providerConfig(srv.URL) + rule(released),
				Check:  resource.TestCheckResourceAttr("linear_git_automation_state.merge", "state_id", released),
			},
			{
				// Import carries the rule's UUID and nothing else, so the read cannot
				// start from the team the way a refresh can — it has to find the team
				// the rule belongs to first.
				ResourceName:      "linear_git_automation_state.merge",
				ImportState:       true,
				ImportStateVerify: true,
				Config:            providerConfig(srv.URL) + rule(released),
			},
			{
				// A rule deleted in Linear has to plan as a recreate, not die at
				// refresh. Reading through a collection makes that this resource's
				// own judgement — nothing came back that is not the same as an
				// EntityNotFoundError until the read says so.
				PreConfig: func() { mock.forget("gitAutomationState") },
				Config:    providerConfig(srv.URL) + rule(released),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							"linear_git_automation_state.merge", plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

// A rule with no state_id means "take no action", overriding an inherited
// default. That null carries meaning, so it has to survive a round-trip rather
// than being quietly dropped from the input.
func TestAccGitAutomationState_nullStateMeansNoAction(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const config = `
resource "linear_git_automation_state" "no_action" {
  team_id = "00000000-0000-4000-8000-000000000999"
  event   = "draft"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check:  resource.TestCheckNoResourceAttr("linear_git_automation_state.no_action", "state_id"),
			},
			{
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
