package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// The `merge` event is the one the community provider cannot round-trip: it
// bundles all five git events into a single resource, so the read cannot tell an
// unset event from a deleted one and a declared `merge` shows up as a permanent
// diff. One resource per event is what fixes that, and this test pins it — all
// five events declared side by side, then an empty plan.
func TestAccGitAutomationState_allFiveEventsRoundTrip(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const config = `
locals {
  team  = "00000000-0000-4000-8000-000000000999"
  state = "00000000-0000-4000-8000-000000000888"
}

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
