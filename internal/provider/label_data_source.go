package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

const issueLabelDataSourceFields = `id name color description isGroup parent { id } team { id }`

// One data source covers both label scopes: unlike the resources, a lookup does
// not have to distinguish them — the read tells you which scope you got by
// whether team_id came back set.

// NewLabelDataSource returns a new linear_label data source.
func NewLabelDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "label",
		kind:     "label",
		schema:   labelDataSourceSchema,
		newModel: func() any { return &labelDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readLabel(ctx, c, model.(*labelDataSourceModel))
		},
	}
}

// NewLabelsDataSource returns a new linear_labels data source.
func NewLabelsDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "labels",
		kind:     "labels",
		schema:   labelsDataSourceSchema,
		newModel: func() any { return &labelsDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readLabels(ctx, c, model.(*labelsDataSourceModel))
		},
	}
}

type labelDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	TeamID      types.String `tfsdk:"team_id"`
	Name        types.String `tfsdk:"name"`
	Color       types.String `tfsdk:"color"`
	Description types.String `tfsdk:"description"`
	IsGroup     types.Bool   `tfsdk:"is_group"`
	ParentID    types.String `tfsdk:"parent_id"`
}

func (m *labelDataSourceModel) fill(a *issueLabelAttributes) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Color = stringOrNull(a.Color)
	m.Description = types.StringPointerValue(a.Description)
	m.IsGroup = types.BoolValue(a.IsGroup)
	m.ParentID = refID(a.Parent)
	m.TeamID = refID(a.Team)
}

func readLabel(ctx context.Context, c *client.Client, m *labelDataSourceModel) error {
	if !m.ID.IsNull() && m.ID.ValueString() != "" {
		var raw json.RawMessage
		e := entity{name: "issueLabel", fields: issueLabelDataSourceFields}
		if err := e.read(ctx, c, m.ID.ValueString(), &raw); err != nil {
			return err
		}
		var a issueLabelAttributes
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		m.fill(&a)
		return nil
	}

	if m.Name.IsNull() || m.Name.ValueString() == "" {
		return fmt.Errorf("set either id or name")
	}

	filter := map[string]any{"name": map[string]any{"eq": m.Name.ValueString()}}
	if !m.TeamID.IsNull() && m.TeamID.ValueString() != "" {
		filter["team"] = map[string]any{"id": map[string]any{"eq": m.TeamID.ValueString()}}
	}

	var labels []issueLabelAttributes
	err := connection(ctx, c, "issueLabels", "$filter: IssueLabelFilter", "filter: $filter",
		issueLabelDataSourceFields, map[string]any{"filter": filter}, &labels)
	if err != nil {
		return err
	}
	if len(labels) == 0 {
		return fmt.Errorf("no label named %q", m.Name.ValueString())
	}
	if len(labels) > 1 {
		return fmt.Errorf("%d labels are named %q — set team_id to say which one", len(labels), m.Name.ValueString())
	}
	m.fill(&labels[0])
	return nil
}

type labelsDataSourceModel struct {
	TeamID types.String           `tfsdk:"team_id"`
	Labels []labelDataSourceModel `tfsdk:"labels"`
}

func readLabels(ctx context.Context, c *client.Client, m *labelsDataSourceModel) error {
	params, args := "", ""
	var vars map[string]any
	if !m.TeamID.IsNull() && m.TeamID.ValueString() != "" {
		params, args = "$filter: IssueLabelFilter", "filter: $filter"
		vars = map[string]any{"filter": map[string]any{
			"team": map[string]any{"id": map[string]any{"eq": m.TeamID.ValueString()}},
		}}
	}

	var labels []issueLabelAttributes
	if err := connection(ctx, c, "issueLabels", params, args, issueLabelDataSourceFields, vars, &labels); err != nil {
		return err
	}
	m.Labels = make([]labelDataSourceModel, 0, len(labels))
	for i := range labels {
		var one labelDataSourceModel
		one.fill(&labels[i])
		m.Labels = append(m.Labels, one)
	}
	return nil
}

func labelDataSourceAttributeSchema(computedOnly bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "UUID of the label.",
			Computed:            true,
		},
		"team_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the team the label is scoped to, null for a workspace label.",
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Name of the label.",
			Computed:            true,
		},
		"color": schema.StringAttribute{
			MarkdownDescription: "Colour of the label as a hex string.",
			Computed:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "Description of the label.",
			Computed:            true,
		},
		"is_group": schema.BoolAttribute{
			MarkdownDescription: "Whether the label is a group other labels nest under.",
			Computed:            true,
		},
		"parent_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the parent label group.",
			Computed:            true,
		},
	}
	if !computedOnly {
		attrs["id"] = schema.StringAttribute{
			MarkdownDescription: "UUID of the label. Set either this or `name`.",
			Optional:            true,
			Computed:            true,
		}
		attrs["name"] = schema.StringAttribute{
			MarkdownDescription: "Name of the label. Set either this or `id`.",
			Optional:            true,
			Computed:            true,
		}
		attrs["team_id"] = schema.StringAttribute{
			MarkdownDescription: "UUID of the team, narrowing a lookup by `name` to that team's labels. Needed " +
				"when a workspace label and a team label share a name.",
			Optional: true,
			Computed: true,
		}
	}
	return attrs
}

func labelDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Looks up an issue label by UUID or name. Covers both workspace and team labels — " +
			"which one you got is visible from whether `team_id` came back set.",
		Attributes: labelDataSourceAttributeSchema(false),
	}
}

func labelsDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Lists issue labels, either of one team or of the whole workspace.",
		Attributes: map[string]schema.Attribute{
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team to list labels for. Leave unset to list every label in " +
					"the workspace.",
				Optional: true,
			},
			"labels": schema.ListNestedAttribute{
				MarkdownDescription: "The labels, as Linear returns them.",
				Computed:            true,
				NestedObject:        schema.NestedAttributeObject{Attributes: labelDataSourceAttributeSchema(true)},
			},
		},
	}
}
