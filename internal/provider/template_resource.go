package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var templateEntity = entity{
	name:   "template",
	fields: `id name description type color icon sortOrder templateData team { id } pipeline { id }`,
}

// NewTemplateResource returns a new linear_template resource.
func NewTemplateResource() resource.Resource {
	return &standardResource{
		entity:   templateEntity,
		typeName: "template",
		kind:     "template",
		schema:   templateSchema,
		newModel: func() crudModel { return &templateModel{} },
	}
}

type templateAttributes struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  *string         `json:"description"`
	Type         string          `json:"type"`
	Color        *string         `json:"color"`
	Icon         *string         `json:"icon"`
	SortOrder    float64         `json:"sortOrder"`
	TemplateData json.RawMessage `json:"templateData"`
	Team         *ref            `json:"team"`
	Pipeline     *ref            `json:"pipeline"`
}

type templateModel struct {
	ID           types.String         `tfsdk:"id"`
	TeamID       types.String         `tfsdk:"team_id"`
	Name         types.String         `tfsdk:"name"`
	Description  types.String         `tfsdk:"description"`
	Type         types.String         `tfsdk:"type"`
	Color        types.String         `tfsdk:"color"`
	Icon         types.String         `tfsdk:"icon"`
	SortOrder    types.Float64        `tfsdk:"sort_order"`
	PipelineID   types.String         `tfsdk:"pipeline_id"`
	TemplateJSON jsontypes.Normalized `tfsdk:"template_json"`
}

func (m *templateModel) id() string { return m.ID.ValueString() }

func (m *templateModel) decode(_ context.Context, raw json.RawMessage) error {
	var a templateAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.Type = types.StringValue(a.Type)
	m.Color = types.StringPointerValue(a.Color)
	m.Icon = types.StringPointerValue(a.Icon)
	m.SortOrder = types.Float64Value(a.SortOrder)
	m.TeamID = refID(a.Team)
	m.PipelineID = refID(a.Pipeline)
	m.TemplateJSON = jsonAttr(a.TemplateData)
	return nil
}

func (m *templateModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}
	putString(in, "description", m.Description, false)
	putString(in, "color", m.Color, false)
	putString(in, "icon", m.Icon, false)
	putFloat(in, "sortOrder", m.SortOrder, false)
	// A null teamId on update is what shares a team template across the whole
	// workspace, so the null goes over explicitly.
	putString(in, "teamId", m.TeamID, forUpdate)
	// putJSON only fails on a malformed document, which the jsontypes.Normalized
	// attribute has already rejected at validate time.
	_ = putJSON(in, "templateData", m.TemplateJSON, false)
	// Neither type nor pipelineId exists on TemplateUpdateInput.
	if !forUpdate {
		in["type"] = m.Type.ValueString()
		putString(in, "pipelineId", m.PipelineID, false)
	}
	return in
}

func templateSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a Linear template — the pre-filled shape new issues, projects or documents " +
			"start from.\n\n" +
			"The template body is deeply nested and its schema follows the entity it templates, so it goes " +
			"through as raw JSON in `template_json` rather than typed HCL.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the template.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the template belongs to. Leave unset to share the template " +
					"across every team in the workspace.",
				Optional: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the template.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the template.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Entity the template applies to, e.g. `issue`, `project`, `document` or " +
					"`releaseNote`. Changing it replaces the template — `templateUpdate` has no `type`.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the template icon as a hex string.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"icon": schema.StringAttribute{
				MarkdownDescription: "Icon of the template.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"sort_order": schema.Float64Attribute{
				MarkdownDescription: "Sort position of the template in the template list. Linear assigns one " +
					"when unset.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepFloat(),
			},
			"pipeline_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the `linear_release_pipeline` the template is bound to. Required " +
					"when `type` is `releaseNote` and rejected otherwise; a pipeline takes at most one release " +
					"note template. Changing it replaces the template.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepString(stringplanmodifier.RequiresReplace()),
			},
			"template_json": schema.StringAttribute{
				MarkdownDescription: "Template body as a JSON object — the pre-filled attributes of the target " +
					"entity, e.g. `jsonencode({ title = \"Incident\", priority = 1 })` for an issue template. " +
					"Compared semantically, so formatting differences are not drift.",
				Required:      true,
				CustomType:    jsontypes.NormalizedType{},
				PlanModifiers: keepJSON(),
			},
		},
	}
}
