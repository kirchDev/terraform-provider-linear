package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var agentSkillEntity = entity{
	name:   "agentSkill",
	fields: `id title body description color icon teamId shared slugId`,
}

// NewAgentSkillResource returns a new linear_agent_skill resource.
func NewAgentSkillResource() resource.Resource {
	return &standardResource{
		entity:   agentSkillEntity,
		typeName: "agent_skill",
		kind:     "agent skill",
		schema:   agentSkillSchema,
		newModel: func() crudModel { return &agentSkillModel{} },
	}
}

type agentSkillAttributes struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	Icon        *string `json:"icon"`
	TeamID      *string `json:"teamId"`
	Shared      bool    `json:"shared"`
	SlugID      string  `json:"slugId"`
}

type agentSkillModel struct {
	ID          types.String `tfsdk:"id"`
	TeamID      types.String `tfsdk:"team_id"`
	Title       types.String `tfsdk:"title"`
	Body        types.String `tfsdk:"body"`
	Description types.String `tfsdk:"description"`
	Color       types.String `tfsdk:"color"`
	Icon        types.String `tfsdk:"icon"`
	Shared      types.Bool   `tfsdk:"shared"`
	SlugID      types.String `tfsdk:"slug_id"`
}

func (m *agentSkillModel) id() string { return m.ID.ValueString() }

func (m *agentSkillModel) decode(_ context.Context, raw json.RawMessage) error {
	var a agentSkillAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Title = types.StringValue(a.Title)
	m.Body = types.StringValue(a.Body)
	m.Description = types.StringPointerValue(a.Description)
	m.Color = types.StringPointerValue(a.Color)
	m.Icon = types.StringPointerValue(a.Icon)
	m.TeamID = types.StringPointerValue(a.TeamID)
	m.Shared = types.BoolValue(a.Shared)
	m.SlugID = types.StringValue(a.SlugID)
	return nil
}

func (m *agentSkillModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"body": m.Body.ValueString()}
	putString(in, "title", m.Title, false)
	putString(in, "color", m.Color, false)
	putString(in, "icon", m.Icon, false)
	// A null teamId shares the skill workspace-wide, so the null goes over
	// explicitly on update.
	putString(in, "teamId", m.TeamID, forUpdate)
	return in
}

func agentSkillSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a Linear agent skill — a named instruction agents can be asked to follow.\n\n" +
			"Scope it to a team via `team_id`, or leave that unset to make it available across the workspace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the skill.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the skill belongs to. Leave unset for a workspace-wide skill.",
				Optional:            true,
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "Title of the skill.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"body": schema.StringAttribute{
				MarkdownDescription: "Instructions the skill carries, as Markdown.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Short description of what the skill does, as Linear derives it.",
				Computed:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the skill icon as a hex string.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"icon": schema.StringAttribute{
				MarkdownDescription: "Icon of the skill.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"shared": schema.BoolAttribute{
				MarkdownDescription: "Whether the skill is shared beyond its owner.",
				Computed:            true,
			},
			"slug_id": schema.StringAttribute{
				MarkdownDescription: "URL slug of the skill.",
				Computed:            true,
			},
		},
	}
}
