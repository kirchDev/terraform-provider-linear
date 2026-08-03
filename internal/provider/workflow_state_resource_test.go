package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// `duplicate` is the whole point of typing workflow_state.type as a string. The
// community provider models it as an enum without that value, which leaves every
// team's Duplicate state unmanageable. This test is here so nobody "tidies" the
// attribute into an enum later.
func TestAccWorkflowState_duplicateTypeIsManageable(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const config = `
resource "linear_workflow_state" "duplicate" {
  team_id = "00000000-0000-4000-8000-000000000999"
  name    = "Duplicate"
  color   = "#95a2b3"
  type    = "duplicate"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_workflow_state.duplicate", "type", "duplicate"),
					resource.TestCheckResourceAttr("linear_workflow_state.duplicate", "name", "Duplicate"),
					resource.TestCheckResourceAttrSet("linear_workflow_state.duplicate", "id"),
				),
			},
			{
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "linear_workflow_state.duplicate",
				ImportState:       true,
				ImportStateVerify: true,
				Config:            providerConfig(srv.URL) + config,
			},
		},
	})
}

// Changing the type or the team has to replace the state: Linear's
// workflowStateUpdate carries neither field, so an in-place update would
// silently do nothing.
func TestAccWorkflowState_typeChangeReplaces(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	state := func(stateType string) string {
		return `
resource "linear_workflow_state" "test" {
  team_id = "00000000-0000-4000-8000-000000000999"
  name    = "In Review"
  color   = "#0f7488"
  type    = "` + stateType + `"
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerConfig(srv.URL) + state("unstarted")},
			{
				Config: providerConfig(srv.URL) + state("started"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("linear_workflow_state.test",
							plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
		},
	})
}

// A name or colour change, by contrast, is an ordinary in-place update.
func TestAccWorkflowState_nameChangeUpdatesInPlace(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	state := func(name string) string {
		return `
resource "linear_workflow_state" "test" {
  team_id = "00000000-0000-4000-8000-000000000999"
  name    = "` + name + `"
  color   = "#0f7488"
  type    = "started"
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerConfig(srv.URL) + state("In Review")},
			{
				Config: providerConfig(srv.URL) + state("Under Review"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("linear_workflow_state.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("linear_workflow_state.test", "name", "Under Review"),
			},
		},
	})
}
