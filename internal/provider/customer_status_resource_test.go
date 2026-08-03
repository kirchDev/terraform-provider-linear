package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// Customer statuses and tiers are the same shape, so one of them standing in
// for both is enough coverage here: create, refresh to an empty plan, update in
// place, import, destroy.
func TestAccCustomerStatus_lifecycle(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	status := func(displayName string) string {
		return `
resource "linear_customer_status" "test" {
  name         = "prospect"
  display_name = "` + displayName + `"
  description  = "In conversation, not yet signed"
  color        = "#bec2c8"
  position     = 1
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + status("Prospect"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("linear_customer_status.test", "id"),
					resource.TestCheckResourceAttr("linear_customer_status.test", "name", "prospect"),
					resource.TestCheckResourceAttr("linear_customer_status.test", "display_name", "Prospect"),
					resource.TestCheckResourceAttr("linear_customer_status.test", "position", "1"),
				),
			},
			{
				Config: providerConfig(srv.URL) + status("Prospect"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: providerConfig(srv.URL) + status("Prospective customer"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("linear_customer_status.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("linear_customer_status.test",
					"display_name", "Prospective customer"),
			},
			{
				ResourceName:      "linear_customer_status.test",
				ImportState:       true,
				ImportStateVerify: true,
				Config:            providerConfig(srv.URL) + status("Prospective customer"),
			},
		},
	})

	if got := mock.count("customerStatus"); got != 0 {
		t.Errorf("mock still holds %d customer statuses after destroy, want 0", got)
	}
}

func TestAccCustomerTier_lifecycle(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const config = `
resource "linear_customer_tier" "enterprise" {
  name         = "enterprise"
  display_name = "Enterprise"
  color        = "#eb5757"
  position     = 1
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_customer_tier.enterprise", "name", "enterprise"),
					resource.TestCheckResourceAttr("linear_customer_tier.enterprise", "display_name", "Enterprise"),
				),
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
