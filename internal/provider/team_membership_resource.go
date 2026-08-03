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

var teamMembershipEntity = entity{
	name:   "teamMembership",
	fields: `id owner sortOrder team { id } user { id }`,
}

// NewTeamMembershipResource returns a new linear_team_membership resource.
func NewTeamMembershipResource() resource.Resource {
	return &standardResource{
		entity:   teamMembershipEntity,
		typeName: "team_membership",
		kind:     "team membership",
		schema:   teamMembershipSchema,
		newModel: func() crudModel { return &teamMembershipModel{} },
	}
}

type teamMembershipAttributes struct {
	ID        string  `json:"id"`
	Owner     bool    `json:"owner"`
	SortOrder float64 `json:"sortOrder"`
	Team      *ref    `json:"team"`
	User      *ref    `json:"user"`
}

type teamMembershipModel struct {
	ID        types.String  `tfsdk:"id"`
	TeamID    types.String  `tfsdk:"team_id"`
	UserID    types.String  `tfsdk:"user_id"`
	Owner     types.Bool    `tfsdk:"owner"`
	SortOrder types.Float64 `tfsdk:"sort_order"`
}

func (m *teamMembershipModel) id() string { return m.ID.ValueString() }

func (m *teamMembershipModel) decode(_ context.Context, raw json.RawMessage) error {
	var a teamMembershipAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Owner = types.BoolValue(a.Owner)
	m.SortOrder = types.Float64Value(a.SortOrder)
	m.TeamID = refID(a.Team)
	m.UserID = refID(a.User)
	return nil
}

func (m *teamMembershipModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{}
	putBool(in, "owner", m.Owner, false)
	putFloat(in, "sortOrder", m.SortOrder, false)
	// Neither teamId nor userId exists on TeamMembershipUpdateInput — moving a
	// membership means deleting one and creating another.
	if !forUpdate {
		in["teamId"] = m.TeamID.ValueString()
		in["userId"] = m.UserID.ValueString()
	}
	return in
}

func teamMembershipSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages the membership of a user in a Linear team, including whether they own it.\n\n" +
			"Destroying the resource removes the user from the team; it does not deactivate the user.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the membership.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team. Changing it replaces the membership.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the user. Changing it replaces the membership.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"owner": schema.BoolAttribute{
				MarkdownDescription: "Whether the user owns the team — team owners may change its settings.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"sort_order": schema.Float64Attribute{
				MarkdownDescription: "Sort position of the team in the user's sidebar.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}
