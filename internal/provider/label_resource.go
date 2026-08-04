package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Workspace labels and team labels are the same Linear entity — issueLabel —
// told apart only by whether teamId is set on create. They get two resource
// types rather than one with an optional team_id because the scope is not
// updatable: issueLabelUpdate has no teamId, so moving a label between workspace
// and team scope is a replace. Two types make that legible in the plan instead
// of surprising at apply.

var issueLabelEntity = entity{
	name:   "issueLabel",
	fields: `id name color description isGroup parent { id } team { id }`,
}

// NewWorkspaceLabelResource returns a new linear_workspace_label resource — a
// label available to every team in the workspace.
func NewWorkspaceLabelResource() resource.Resource {
	return &standardResource{
		entity:   issueLabelEntity,
		typeName: "workspace_label",
		kind:     "workspace label",
		schema:   func() schema.Schema { return issueLabelSchema(false) },
		newModel: func() crudModel { return &issueLabelModel{} },
	}
}

// NewTeamLabelResource returns a new linear_team_label resource — a label
// scoped to a single team.
func NewTeamLabelResource() resource.Resource {
	return &standardResource{
		entity:   issueLabelEntity,
		typeName: "team_label",
		kind:     "team label",
		schema:   func() schema.Schema { return issueLabelSchema(true) },
		newModel: func() crudModel { return &issueLabelModel{} },
	}
}

// issueLabelAttributes mirrors Linear's IssueLabel.
type issueLabelAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
	IsGroup     bool    `json:"isGroup"`
	Parent      *ref    `json:"parent"`
	Team        *ref    `json:"team"`
}

// issueLabelModel is the state of both label resources. TeamID is computed for a
// workspace label (always null) and required for a team label.
type issueLabelModel struct {
	ID          types.String `tfsdk:"id"`
	TeamID      types.String `tfsdk:"team_id"`
	Name        types.String `tfsdk:"name"`
	Color       types.String `tfsdk:"color"`
	Description types.String `tfsdk:"description"`
	IsGroup     types.Bool   `tfsdk:"is_group"`
	ParentID    types.String `tfsdk:"parent_id"`
}

func (m *issueLabelModel) id() string { return m.ID.ValueString() }

func (m *issueLabelModel) decode(_ context.Context, raw json.RawMessage) error {
	var a issueLabelAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Color = stringOrNull(a.Color)
	m.Description = types.StringPointerValue(a.Description)
	m.IsGroup = types.BoolValue(a.IsGroup)
	m.ParentID = refID(a.Parent)
	m.TeamID = refID(a.Team)
	return nil
}

func (m *issueLabelModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}
	// Optional + Computed throughout, so clear=false: an attribute the
	// configuration omits keeps its live value instead of being nulled.
	putString(in, "color", m.Color, false)
	putString(in, "description", m.Description, false)
	putString(in, "parentId", m.ParentID, false)
	// isGroup and teamId only exist on the create input.
	if !forUpdate {
		putBool(in, "isGroup", m.IsGroup, false)
		putString(in, "teamId", m.TeamID, false)
	}
	return in
}

func issueLabelSchema(teamScoped bool) schema.Schema {
	scope := "workspace"
	if teamScoped {
		scope = "team"
	}

	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "UUID of the label.",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Name of the label. Unique within its " + scope + ".",
			Required:            true,
		},
		"color": schema.StringAttribute{
			MarkdownDescription: "Colour of the label as a hex string, e.g. `#5e6ad2`. Linear picks one when unset.",
			Optional:            true,
			Computed:            true,
			PlanModifiers:       keepString(),
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "Description of the label.",
			Optional:            true,
			Computed:            true,
		},
		"is_group": schema.BoolAttribute{
			MarkdownDescription: "Whether the label is a group — a container other labels nest under, which cannot " +
				"be applied to an issue itself. Changing this replaces the label.",
			Optional:      true,
			Computed:      true,
			Default:       booldefault.StaticBool(false),
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
		},
		"parent_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the parent label group this label nests under.",
			Optional:            true,
			Computed:            true,
		},
	}

	if teamScoped {
		attrs["team_id"] = schema.StringAttribute{
			MarkdownDescription: "UUID of the team the label belongs to. Changing it replaces the label — " +
				"`issueLabelUpdate` has no `teamId`.",
			Required:      true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		}
		return schema.Schema{
			MarkdownDescription: "Manages an issue label scoped to a single Linear team.\n\n" +
				"For a label available to every team, use `linear_workspace_label`. The scope is fixed at " +
				"creation — Linear has no mutation that moves a label between team and workspace scope.",
			Attributes: attrs,
		}
	}

	attrs["team_id"] = schema.StringAttribute{
		MarkdownDescription: "Always null — a workspace label belongs to no team. Present so the attribute set " +
			"matches `linear_team_label`.",
		Computed: true,
	}
	return schema.Schema{
		MarkdownDescription: "Manages a workspace-wide issue label in Linear — available to every team.\n\n" +
			"For a label scoped to a single team, use `linear_team_label`. The scope is fixed at creation — " +
			"Linear has no mutation that moves a label between workspace and team scope.",
		Attributes: attrs,
	}
}
