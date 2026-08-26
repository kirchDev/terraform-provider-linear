package provider

import (
	"fmt"
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

// Sending only what changed is a property of the shared Update every standard
// resource runs, not of linear_team — the resource it was reported against
// happens to be the widest one, which is why its echo was the one Linear
// refused. A label is the same construction with five attributes instead of
// forty: Optional + Computed throughout, so an attribute the configuration never
// mentions carries its live value into the plan and would be echoed straight
// back.
//
// This is here rather than beside the team test so a fix that landed on the
// resource instead of on the machinery shows up as a failure rather than as a
// second report.
func TestAccWorkspaceLabel_updateSendsOnlyWhatChanged(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const described = `
resource "linear_workspace_label" "test" {
  name        = "Bug"
  color       = "#d73a4a"
  description = "Something isn't working"
}
`
	const recoloured = `
resource "linear_workspace_label" "test" {
  name        = "Bug"
  color       = "#b60205"
  description = "Something isn't working"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerConfig(srv.URL) + described},
			{
				Config: providerConfig(srv.URL) + recoloured,
				Check: func(*terraform.State) error {
					inputs := mock.updateInputs("issueLabel")
					if len(inputs) != 1 {
						return fmt.Errorf("want exactly one issueLabelUpdate, got %d: %#v", len(inputs), inputs)
					}
					in := inputs[0]
					if got := in["color"]; got != "#b60205" {
						return fmt.Errorf("update did not carry the changed colour, got %#v", got)
					}
					if len(in) != 1 {
						return fmt.Errorf("update echoed %d unchanged field(s): %#v", len(in)-1, in)
					}
					return nil
				},
			},
		},
	})
}

// Reading back after the mutation is a property of the shared Update too, and
// this is the half `updateSendsOnlyWhatChanged` above cannot see: that one
// asserts on what went out, this one on what came back.
//
// Linear answers an xUpdate with the entity, and the provider used to decode
// that answer straight into state. Where the answer has not caught up with the
// write it carries the *old* value — the mutation succeeded, the API reports
// the new value a moment later, and Terraform still refuses the apply with
// "Provider produced inconsistent result after apply" because the value handed
// back is not the value it planned. The change lands and the run fails, which
// is the worst of both.
//
// A label rather than the workspace, deliberately: the report came from
// linear_workspace_settings, whose Update is hand-written, and the same decode
// sits in standardResource where every other resource runs it.
func TestAccWorkspaceLabel_updateReadsBackWhenTheMutationResponseLags(t *testing.T) {
	mock := newLinearMock()
	srv := mock.server(t)

	const described = `
resource "linear_workspace_label" "test" {
  name  = "Bug"
  color = "#d73a4a"
}
`
	const recoloured = `
resource "linear_workspace_label" "test" {
  name  = "Bug"
  color = "#b60205"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerConfig(srv.URL) + described},
			{
				// Only the update under test lags; the create above is left alone,
				// so what this asserts on is the update path and nothing else.
				PreConfig: func() { mock.lagUpdate("issueLabel") },
				Config:    providerConfig(srv.URL) + recoloured,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("linear_workspace_label.test", "color", "#b60205"),
					func(*terraform.State) error {
						// The mutation is still the thing that carries the change:
						// a "fix" that stopped sending it and only read would pass
						// the assertion above on a workspace nothing had changed.
						inputs := mock.updateInputs("issueLabel")
						if len(inputs) != 1 {
							return fmt.Errorf("want exactly one issueLabelUpdate, got %d: %#v", len(inputs), inputs)
						}
						if got := inputs[0]["color"]; got != "#b60205" {
							return fmt.Errorf("update did not carry the changed colour, got %#v", got)
						}
						return nil
					},
				),
			},
		},
	})
}
