package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// The view round-trip is the test that matters most in this provider. Linear
// normalises filterData server-side, so the JSON coming back is not the JSON
// that went out. If filter_json compared byte for byte, every plan after the
// first would show a diff — which is exactly what stalled the upstream pull
// request for views. The mock re-serialises JSON on the way in to reproduce
// that, and the assertion below is that the plan is nevertheless empty.
func TestAccCustomView_filterRoundTrip(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	// Written with the keys out of alphabetical order and with nesting, so the
	// mock's re-serialisation genuinely changes the bytes.
	const config = `
resource "linear_custom_view" "test" {
  name        = "In Review"
  description = "Everything waiting on a reviewer"
  shared      = true

  filter_json = jsonencode({
    state  = { type = { eq = "started" } }
    labels = { some = { name = { eq = "ai" } } }
    assignee = { null = false }
  })
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("linear_custom_view.test", "id"),
					resource.TestCheckResourceAttr("linear_custom_view.test", "name", "In Review"),
					resource.TestCheckResourceAttr("linear_custom_view.test", "shared", "true"),
				),
			},
			{
				// The refresh-and-plan the framework runs here is the actual
				// assertion: a semantic comparison keeps it empty, a byte
				// comparison would not.
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:      "linear_custom_view.test",
				ImportState:       true,
				ImportStateVerify: true,
				Config:            providerConfig(srv.URL) + config,
			},
		},
	})
}

// A view whose filter changes should update in place rather than being
// replaced, and the new filter should be what comes back.
func TestAccCustomView_updateFilter(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	view := func(labelName string) string {
		return fmt.Sprintf(`
resource "linear_custom_view" "test" {
  name = "In Review"

  filter_json = jsonencode({
    labels = { some = { name = { eq = %q } } }
  })
}
`, labelName)
	}

	var firstID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + view("ai"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("linear_custom_view.test", "id", func(v string) error {
						firstID = v
						return nil
					}),
				),
			},
			{
				Config: providerConfig(srv.URL) + view("infra"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("linear_custom_view.test", "id", func(v string) error {
						if v != firstID {
							return fmt.Errorf("view was replaced (%s → %s); a filter change is an in-place update", firstID, v)
						}
						return nil
					}),
					resource.TestCheckResourceAttrWith("linear_custom_view.test", "filter_json", func(v string) error {
						if !strings.Contains(v, "infra") {
							return fmt.Errorf("filter_json did not pick up the new label: %s", v)
						}
						return nil
					}),
				),
			},
		},
	})

	// resource.Test destroys everything at the end of the case, so what is left
	// upstream says whether the update replaced the view behind a delete or
	// really did update it in place.
	if got := mock.count("customView"); got != 0 {
		t.Errorf("mock still holds %d views after destroy, want 0", got)
	}
}

// Destroying the resource has to actually delete it upstream — and a resource
// deleted outside Terraform has to leave state on the next refresh rather than
// failing the plan.
func TestAccCustomView_destroyAndDriftRemoval(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const config = `
resource "linear_custom_view" "test" {
  name        = "Scratch"
  filter_json = jsonencode({ state = { type = { eq = "backlog" } } })
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerConfig(srv.URL) + config},
			{
				// Delete it behind Terraform's back. Linear reports the missing
				// entity as EntityNotFoundError inside an HTTP 200; the refresh has
				// to read that as "gone" and plan a create, not error out.
				PreConfig: func() {
					mock.mu.Lock()
					mock.entities["customView"] = map[string]map[string]any{}
					mock.mu.Unlock()
				},
				Config: providerConfig(srv.URL) + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("linear_custom_view.test", plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})

	if got := mock.count("customView"); got != 0 {
		t.Errorf("mock still holds %d views after destroy, want 0", got)
	}
}
