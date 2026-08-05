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

// Project labels are their own entity with their own mutations, not issue labels
// applied to projects — same semantics (groups, nesting, colour), separate
// namespace.
var projectLabelEntity = entity{
	name:   "projectLabel",
	fields: `id name color description isGroup parent { id } team { id }`,
}

// NewProjectLabelResource returns a new linear_project_label resource.
func NewProjectLabelResource() resource.Resource {
	return &standardResource{
		entity:   projectLabelEntity,
		typeName: "project_label",
		kind:     "project label",
		schema:   func() schema.Schema { return groupableLabelSchema("project", true) },
		newModel: func() crudModel { return &projectLabelModel{} },
	}
}

type projectLabelModel struct {
	ID          types.String `tfsdk:"id"`
	TeamID      types.String `tfsdk:"team_id"`
	Name        types.String `tfsdk:"name"`
	Color       types.String `tfsdk:"color"`
	Description types.String `tfsdk:"description"`
	IsGroup     types.Bool   `tfsdk:"is_group"`
	ParentID    types.String `tfsdk:"parent_id"`
}

func (m *projectLabelModel) id() string { return m.ID.ValueString() }

func (m *projectLabelModel) decode(_ context.Context, raw json.RawMessage) error {
	var a issueLabelAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Color = stringOrNull(a.Color)
	m.Description = types.StringPointerValue(a.Description)
	m.IsGroup = types.BoolValue(a.IsGroup)
	m.ParentID = keepCleared(m.ParentID, refID(a.Parent))
	m.TeamID = refID(a.Team)
	return nil
}

func (m *projectLabelModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}
	// Optional + Computed throughout, so clear=false: an attribute the
	// configuration omits keeps its live value instead of being nulled.
	putString(in, "color", m.Color, false)
	putString(in, "description", m.Description, false)
	putRef(in, "parentId", m.ParentID)
	if !forUpdate {
		putBool(in, "isGroup", m.IsGroup, false)
		putString(in, "teamId", m.TeamID, false)
	}
	return in
}

// groupableLabelSchema is shared by project and initiative labels — Linear's
// three label entities have the same shape, differing only in whether a label
// can be scoped to a team.
func groupableLabelSchema(kind string, teamScoped bool) schema.Schema {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "UUID of the label.",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Name of the label.",
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
			PlanModifiers:       keepString(),
		},
		"is_group": schema.BoolAttribute{
			MarkdownDescription: "Whether the label is a group other labels nest under. Changing this replaces " +
				"the label.",
			Optional:      true,
			Computed:      true,
			Default:       booldefault.StaticBool(false),
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
		},
		"parent_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the parent label group this label nests under." + clearWithEmptyString,
			Optional:            true,
			Computed:            true,
			PlanModifiers:       keepString(),
		},
	}

	desc := "Manages a " + kind + " label in Linear. " + kind + " labels are their own entity with their own " +
		"mutations — they are not issue labels applied to " + kind + "s."
	if teamScoped {
		attrs["team_id"] = schema.StringAttribute{
			MarkdownDescription: "UUID of the team the label is scoped to. Leave unset for a workspace-wide " +
				"label. Changing it replaces the label — the update mutation has no `teamId`.",
			Optional:      true,
			Computed:      true,
			PlanModifiers: keepString(stringplanmodifier.RequiresReplace()),
		}
	}
	return schema.Schema{MarkdownDescription: desc, Attributes: attrs}
}
