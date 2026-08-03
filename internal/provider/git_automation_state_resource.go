package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

// One resource per git event, not one bundling all five. GitAutomationState
// carries a single `event`, and modelling the five as attributes of one resource
// is what stops `merge` round-tripping in the community provider: the read maps
// five entities onto one object and cannot tell an unset event from a deleted
// one, so a declared `merge` shows up as a permanent diff.
var gitAutomationStateEntity = entity{
	name:   "gitAutomationState",
	fields: `id event state { id } targetBranch { id } team { id }`,
}

// NewGitAutomationStateResource returns a new linear_git_automation_state resource.
func NewGitAutomationStateResource() resource.Resource {
	return &standardResource{
		entity:   gitAutomationStateEntity,
		typeName: "git_automation_state",
		kind:     "git automation state",
		schema:   gitAutomationStateSchema,
		newModel: func() crudModel { return &gitAutomationStateModel{} },
	}
}

type gitAutomationStateAttributes struct {
	ID           string `json:"id"`
	Event        string `json:"event"`
	State        *ref   `json:"state"`
	TargetBranch *ref   `json:"targetBranch"`
	Team         *ref   `json:"team"`
}

type gitAutomationStateModel struct {
	ID             types.String `tfsdk:"id"`
	TeamID         types.String `tfsdk:"team_id"`
	Event          types.String `tfsdk:"event"`
	StateID        types.String `tfsdk:"state_id"`
	TargetBranchID types.String `tfsdk:"target_branch_id"`
}

func (m *gitAutomationStateModel) id() string { return m.ID.ValueString() }

func (m *gitAutomationStateModel) decode(_ context.Context, raw json.RawMessage) error {
	var a gitAutomationStateAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Event = types.StringValue(a.Event)
	m.StateID = refID(a.State)
	m.TargetBranchID = refID(a.TargetBranch)
	m.TeamID = refID(a.Team)
	return nil
}

func (m *gitAutomationStateModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"event": m.Event.ValueString()}
	// Both nulls carry meaning here, so they are sent explicitly on update:
	// stateId null means "take no action, overriding the default rule", and
	// targetBranchId null means "apply to every branch".
	putString(in, "stateId", m.StateID, forUpdate)
	putString(in, "targetBranchId", m.TargetBranchID, forUpdate)
	if !forUpdate {
		in["teamId"] = m.TeamID.ValueString()
	}
	return in
}

func gitAutomationStateSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages one git automation rule of a Linear team: when the given git event fires, " +
			"linked issues move to the given workflow state.\n\n" +
			"Each Linear `GitAutomationState` covers exactly one event, so five events means five resources. " +
			"Combined with `linear_git_automation_target_branch` this also expresses per-branch overrides — a rule " +
			"with `target_branch_id` set applies only to pull requests targeting that branch pattern and overrides " +
			"the team's default rule for the same event.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the automation rule.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the rule belongs to. Changing it replaces the rule — " +
					"`gitAutomationStateUpdate` has no `teamId`.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"event": schema.StringAttribute{
				MarkdownDescription: "Git event that triggers the rule — one of `draft` (draft PR opened), " +
					"`start` (branch created), `review` (PR ready for review), `mergeable` (PR approved and " +
					"mergeable) or `merge` (PR merged).",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("draft", "start", "review", "mergeable", "merge"),
				},
			},
			"state_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the workflow state linked issues move to. Leave unset to make the " +
					"rule take no action — which is how a team overrides an inherited default for this event.",
				Optional: true,
			},
			"target_branch_id": schema.StringAttribute{
				MarkdownDescription: "UUID of a `linear_git_automation_target_branch` this rule is scoped to. " +
					"Unset means the rule applies to pull requests against any branch.",
				Optional: true,
			},
		},
	}
}
