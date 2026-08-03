package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// NewTemplateDataSource returns a new linear_template data source.
func NewTemplateDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "template",
		kind:     "template",
		schema:   templateDataSourceSchema,
		newModel: func() any { return &templateDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readTemplate(ctx, c, model.(*templateDataSourceModel))
		},
	}
}

type templateDataSourceModel struct {
	ID           types.String         `tfsdk:"id"`
	TeamID       types.String         `tfsdk:"team_id"`
	Name         types.String         `tfsdk:"name"`
	Description  types.String         `tfsdk:"description"`
	Type         types.String         `tfsdk:"type"`
	Color        types.String         `tfsdk:"color"`
	Icon         types.String         `tfsdk:"icon"`
	SortOrder    types.Float64        `tfsdk:"sort_order"`
	TemplateJSON jsontypes.Normalized `tfsdk:"template_json"`
}

func readTemplate(ctx context.Context, c *client.Client, m *templateDataSourceModel) error {
	const fields = `id name description type color icon sortOrder templateData team { id }`

	if m.ID.IsNull() || m.ID.ValueString() == "" {
		// Linear has no templates() collection to search — templatesForIntegration
		// is the only list query and it is scoped to an integration — so a lookup
		// needs the id.
		return fmt.Errorf("set id: Linear offers no template collection to search by name")
	}

	var raw json.RawMessage
	if err := (entity{name: "template", fields: fields}).read(ctx, c, m.ID.ValueString(), &raw); err != nil {
		return err
	}
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
	m.TemplateJSON = jsonAttr(a.TemplateData)
	return nil
}

func templateDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Reads a Linear template by UUID.\n\n" +
			"There is no lookup by name: Linear exposes no template collection to search, only " +
			"`templatesForIntegration`, which is scoped to an integration rather than the workspace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the template.",
				Required:            true,
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the template belongs to, null when it is shared across " +
					"the workspace.",
				Computed: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the template.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the template.",
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Entity the template applies to, e.g. `issue` or `project`.",
				Computed:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the template icon as a hex string.",
				Computed:            true,
			},
			"icon": schema.StringAttribute{
				MarkdownDescription: "Icon of the template.",
				Computed:            true,
			},
			"sort_order": schema.Float64Attribute{
				MarkdownDescription: "Sort position of the template in the template list.",
				Computed:            true,
			},
			"template_json": schema.StringAttribute{
				MarkdownDescription: "Template body as Linear stores it.",
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
			},
		},
	}
}
