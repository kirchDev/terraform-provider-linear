package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The empty-string clear is a property of the provider rather than of
// linear_team, so it is checked somewhere other than the resource it was
// reported against. A label's parent_id is the same shape — a UUID-typed
// Optional + Computed reference — and it goes through the same two helpers, so a
// call site that was converted on one resource and missed on another shows up
// here rather than in a practitioner's plan.
//
// Nesting a label under a group and then taking it back out is the whole of what
// this has to support, and before the sentinel there was no configuration that
// could express the second half.
func TestAccWorkspaceLabel_emptyStringClearsTheParent(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const group = "7c2ad914-0000-4000-8000-000000000009"

	const nested = `
resource "linear_workspace_label" "test" {
  name      = "Bug"
  parent_id = "` + group + `"
}
`

	const cleared = `
resource "linear_workspace_label" "test" {
  name      = "Bug"
  parent_id = ""
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + nested,
				Check:  resource.TestCheckResourceAttr("linear_workspace_label.test", "parent_id", group),
			},
			{
				Config: providerConfig(srv.URL) + cleared,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_workspace_label.test", "parent_id", ""),
					func(*terraform.State) error {
						stored := mock.only(t, "issueLabel")
						for _, field := range []string{"parent", "parentId"} {
							if got, ok := stored[field]; ok {
								t.Errorf("label still holds %s = %#v: the update sent an empty string "+
									"rather than an explicit null", field, got)
							}
						}
						return nil
					},
				),
			},
			{
				Config: providerConfig(srv.URL) + cleared,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("linear_workspace_label.test", "parent_id", ""),
			},
		},
	})
}
