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

const customViewDataSourceFields = `id name description icon color shared modelName
	filterData projectFilterData initiativeFilterData feedItemFilterData
	team { id } owner { id }`

// NewCustomViewDataSource returns a new linear_custom_view data source.
func NewCustomViewDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "custom_view",
		kind:     "custom view",
		schema:   customViewDataSourceSchema,
		newModel: func() any { return &customViewDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readCustomView(ctx, c, model.(*customViewDataSourceModel))
		},
	}
}

// NewCustomViewsDataSource returns a new linear_custom_views data source.
func NewCustomViewsDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "custom_views",
		kind:     "custom views",
		schema:   customViewsDataSourceSchema,
		newModel: func() any { return &customViewsDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readCustomViews(ctx, c, model.(*customViewsDataSourceModel))
		},
	}
}

type customViewDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Icon        types.String `tfsdk:"icon"`
	Color       types.String `tfsdk:"color"`
	Shared      types.Bool   `tfsdk:"shared"`
	ModelName   types.String `tfsdk:"model_name"`
	TeamID      types.String `tfsdk:"team_id"`
	OwnerID     types.String `tfsdk:"owner_id"`

	FilterJSON           jsontypes.Normalized `tfsdk:"filter_json"`
	ProjectFilterJSON    jsontypes.Normalized `tfsdk:"project_filter_json"`
	InitiativeFilterJSON jsontypes.Normalized `tfsdk:"initiative_filter_json"`
	FeedItemFilterJSON   jsontypes.Normalized `tfsdk:"feed_item_filter_json"`
}

func (m *customViewDataSourceModel) fill(a *customViewAttributes) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.Icon = types.StringPointerValue(a.Icon)
	m.Color = types.StringPointerValue(a.Color)
	m.Shared = types.BoolValue(a.Shared)
	m.ModelName = types.StringValue(a.ModelName)
	m.TeamID = refID(a.Team)
	m.OwnerID = refID(a.Owner)
	m.FilterJSON = jsonAttr(a.FilterData)
	m.ProjectFilterJSON = jsonAttr(a.ProjectFilterData)
	m.InitiativeFilterJSON = jsonAttr(a.InitiativeFilterData)
	m.FeedItemFilterJSON = jsonAttr(a.FeedItemFilterData)
}

func readCustomView(ctx context.Context, c *client.Client, m *customViewDataSourceModel) error {
	if !m.ID.IsNull() && m.ID.ValueString() != "" {
		var raw json.RawMessage
		e := entity{name: "customView", fields: customViewDataSourceFields}
		if err := e.read(ctx, c, m.ID.ValueString(), &raw); err != nil {
			return err
		}
		var a customViewAttributes
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		m.fill(&a)
		return nil
	}

	if m.Name.IsNull() || m.Name.ValueString() == "" {
		return fmt.Errorf("set either id or name")
	}

	// CustomViewFilter has no name comparator worth relying on, so the match
	// happens here over the full list. Workspaces have tens of views, not
	// thousands, and the connection helper pages through all of them.
	var views []customViewAttributes
	if err := connection(ctx, c, "customViews", "", "", customViewDataSourceFields, nil, &views); err != nil {
		return err
	}
	var matches []customViewAttributes
	for _, v := range views {
		if v.Name == m.Name.ValueString() {
			matches = append(matches, v)
		}
	}
	switch len(matches) {
	case 0:
		return fmt.Errorf("no custom view named %q", m.Name.ValueString())
	case 1:
		m.fill(&matches[0])
		return nil
	default:
		return fmt.Errorf("%d custom views are named %q — look it up by id instead", len(matches), m.Name.ValueString())
	}
}

type customViewsDataSourceModel struct {
	Views []customViewDataSourceModel `tfsdk:"custom_views"`
}

func readCustomViews(ctx context.Context, c *client.Client, m *customViewsDataSourceModel) error {
	var views []customViewAttributes
	if err := connection(ctx, c, "customViews", "", "", customViewDataSourceFields, nil, &views); err != nil {
		return err
	}
	m.Views = make([]customViewDataSourceModel, 0, len(views))
	for i := range views {
		var one customViewDataSourceModel
		one.fill(&views[i])
		m.Views = append(m.Views, one)
	}
	return nil
}

func customViewDataSourceAttributeSchema(computedOnly bool) map[string]schema.Attribute {
	jsonAttr := func(desc string) schema.Attribute {
		return schema.StringAttribute{
			MarkdownDescription: desc,
			Computed:            true,
			CustomType:          jsontypes.NormalizedType{},
		}
	}

	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "UUID of the view.",
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Name of the view.",
			Computed:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "Description of the view.",
			Computed:            true,
		},
		"icon": schema.StringAttribute{
			MarkdownDescription: "Icon of the view.",
			Computed:            true,
		},
		"color": schema.StringAttribute{
			MarkdownDescription: "Colour of the view icon as a hex string.",
			Computed:            true,
		},
		"shared": schema.BoolAttribute{
			MarkdownDescription: "Whether the view is visible to the whole workspace.",
			Computed:            true,
		},
		"model_name": schema.StringAttribute{
			MarkdownDescription: "What the view lists — `Issue`, `Project`, `Initiative` or `FeedItem`.",
			Computed:            true,
		},
		"team_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the team the view is scoped to, null for a workspace-wide view.",
			Computed:            true,
		},
		"owner_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the user who owns the view.",
			Computed:            true,
		},
		"filter_json":            jsonAttr("Issue filter as Linear stores it, normalised."),
		"project_filter_json":    jsonAttr("Project filter as Linear stores it, normalised."),
		"initiative_filter_json": jsonAttr("Initiative filter as Linear stores it, normalised."),
		"feed_item_filter_json":  jsonAttr("Feed item filter as Linear stores it, normalised."),
	}

	if !computedOnly {
		attrs["id"] = schema.StringAttribute{
			MarkdownDescription: "UUID of the view. Set either this or `name`.",
			Optional:            true,
			Computed:            true,
		}
		attrs["name"] = schema.StringAttribute{
			MarkdownDescription: "Name of the view. Set either this or `id`. Ambiguous when several views share " +
				"a name — look those up by id.",
			Optional: true,
			Computed: true,
		}
	}
	return attrs
}

func customViewDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Looks up a Linear custom view by UUID or name.\n\n" +
			"Reading the filter of an existing view is the quickest way to work out what to put in a " +
			"`linear_custom_view` resource's `filter_json`.",
		Attributes: customViewDataSourceAttributeSchema(false),
	}
}

func customViewsDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Lists every Linear custom view the API key can see.",
		Attributes: map[string]schema.Attribute{
			"custom_views": schema.ListNestedAttribute{
				MarkdownDescription: "The views, as Linear returns them.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: customViewDataSourceAttributeSchema(true),
				},
			},
		},
	}
}
