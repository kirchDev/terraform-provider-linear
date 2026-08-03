package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Project statuses are the same class of thing as workflow states, one level up:
// they describe where a project stands. They are workspace-wide, not per team,
// and Linear archives rather than deletes them.
var projectStatusEntity = entity{
	name:       "projectStatus",
	fields:     `id name description color type position indefinite`,
	deleteVerb: "Archive",
}

// NewProjectStatusResource returns a new linear_project_status resource.
func NewProjectStatusResource() resource.Resource {
	return &standardResource{
		entity:    projectStatusEntity,
		typeName:  "project_status",
		kind:      "project status",
		schema:    projectStatusSchema,
		newModel:  func() crudModel { return &projectStatusModel{} },
		deleteMsg: "Unable to archive Linear project status",
	}
}

type projectStatusAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Color       string  `json:"color"`
	Type        string  `json:"type"`
	Position    float64 `json:"position"`
	Indefinite  bool    `json:"indefinite"`
}

type projectStatusModel struct {
	ID          types.String  `tfsdk:"id"`
	Name        types.String  `tfsdk:"name"`
	Description types.String  `tfsdk:"description"`
	Color       types.String  `tfsdk:"color"`
	Type        types.String  `tfsdk:"type"`
	Position    types.Float64 `tfsdk:"position"`
	Indefinite  types.Bool    `tfsdk:"indefinite"`
}

func (m *projectStatusModel) id() string { return m.ID.ValueString() }

func (m *projectStatusModel) decode(_ context.Context, raw json.RawMessage) error {
	var a projectStatusAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.Color = types.StringValue(a.Color)
	m.Type = types.StringValue(a.Type)
	m.Position = types.Float64Value(a.Position)
	m.Indefinite = types.BoolValue(a.Indefinite)
	return nil
}

func (m *projectStatusModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{
		"name":     m.Name.ValueString(),
		"color":    m.Color.ValueString(),
		"type":     m.Type.ValueString(),
		"position": m.Position.ValueFloat64(),
	}
	putString(in, "description", m.Description, forUpdate)
	putBool(in, "indefinite", m.Indefinite, false)
	return in
}

func projectStatusSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a project status in Linear — where a project stands, workspace-wide.\n\n" +
			"Destroying the resource archives the status; Linear has no hard delete for project statuses.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the project status.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the status, e.g. `In Progress`.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the status.",
				Optional:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the status indicator as a hex string.",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Category the status belongs to — one of `backlog`, `planned`, `started`, " +
					"`paused`, `completed`, `canceled`.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("backlog", "planned", "started", "paused", "completed", "canceled"),
				},
			},
			"position": schema.Float64Attribute{
				MarkdownDescription: "Sort position of the status within its category.",
				Required:            true,
			},
			"indefinite": schema.BoolAttribute{
				MarkdownDescription: "Whether a project may sit in this status indefinitely, so Linear does not " +
					"nag for an update.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}
