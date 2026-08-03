package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A target branch is its own entity rather than a field on the automation rule,
// which is what lets a team have several branch patterns, each with its own set
// of per-event rules.
var gitAutomationTargetBranchEntity = entity{
	name:   "gitAutomationTargetBranch",
	fields: `id branchPattern isRegex team { id }`,
}

// NewGitAutomationTargetBranchResource returns a new
// linear_git_automation_target_branch resource.
func NewGitAutomationTargetBranchResource() resource.Resource {
	return &standardResource{
		entity:   gitAutomationTargetBranchEntity,
		typeName: "git_automation_target_branch",
		kind:     "git automation target branch",
		schema:   gitAutomationTargetBranchSchema,
		newModel: func() crudModel { return &gitAutomationTargetBranchModel{} },
	}
}

type gitAutomationTargetBranchAttributes struct {
	ID            string `json:"id"`
	BranchPattern string `json:"branchPattern"`
	IsRegex       bool   `json:"isRegex"`
	Team          *ref   `json:"team"`
}

type gitAutomationTargetBranchModel struct {
	ID            types.String `tfsdk:"id"`
	TeamID        types.String `tfsdk:"team_id"`
	BranchPattern types.String `tfsdk:"branch_pattern"`
	IsRegex       types.Bool   `tfsdk:"is_regex"`
}

func (m *gitAutomationTargetBranchModel) id() string { return m.ID.ValueString() }

func (m *gitAutomationTargetBranchModel) decode(_ context.Context, raw json.RawMessage) error {
	var a gitAutomationTargetBranchAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.BranchPattern = types.StringValue(a.BranchPattern)
	m.IsRegex = types.BoolValue(a.IsRegex)
	m.TeamID = refID(a.Team)
	return nil
}

func (m *gitAutomationTargetBranchModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"branchPattern": m.BranchPattern.ValueString()}
	putBool(in, "isRegex", m.IsRegex, false)
	if !forUpdate {
		in["teamId"] = m.TeamID.ValueString()
	}
	return in
}

func gitAutomationTargetBranchSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a target branch pattern for a Linear team's git automation. Reference it " +
			"from `linear_git_automation_state.target_branch_id` to scope automation rules to pull requests " +
			"against branches matching this pattern.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the target branch definition.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the pattern belongs to. Changing it replaces the resource — " +
					"`gitAutomationTargetBranchUpdate` has no `teamId`.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"branch_pattern": schema.StringAttribute{
				MarkdownDescription: "Branch name to match, e.g. `main`. Unique within the team. Treated as a " +
					"regular expression when `is_regex` is true.",
				Required: true,
			},
			"is_regex": schema.BoolAttribute{
				MarkdownDescription: "Whether `branch_pattern` is a regular expression rather than an exact " +
					"branch name.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}
