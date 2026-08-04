package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// NewOrganizationDataSource returns a new linear_organization data source.
func NewOrganizationDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "organization",
		kind:     "organization",
		schema:   organizationDataSourceSchema,
		newModel: func() any { return &organizationDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readOrganizationDataSource(ctx, c, model.(*organizationDataSourceModel))
		},
	}
}

type organizationDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	URLKey            types.String `tfsdk:"url_key"`
	LogoURL           types.String `tfsdk:"logo_url"`
	UserCount         types.Int64  `tfsdk:"user_count"`
	CreatedIssueCount types.Int64  `tfsdk:"created_issue_count"`
	RoadmapEnabled    types.Bool   `tfsdk:"roadmap_enabled"`
	ReleasesEnabled   types.Bool   `tfsdk:"releases_enabled"`
	CustomersEnabled  types.Bool   `tfsdk:"customers_enabled"`
	SAMLEnabled       types.Bool   `tfsdk:"saml_enabled"`
	SCIMEnabled       types.Bool   `tfsdk:"scim_enabled"`
}

func readOrganizationDataSource(ctx context.Context, c *client.Client, m *organizationDataSourceModel) error {
	const fields = `id name urlKey logoUrl userCount createdIssueCount
		roadmapEnabled releasesEnabled customersEnabled samlEnabled scimEnabled`

	doc := "query organization {\n  organization { " + fields + " }\n}"
	var data map[string]json.RawMessage
	if err := c.Query(ctx, doc, nil, &data); err != nil {
		return err
	}
	var a struct {
		ID                string  `json:"id"`
		Name              string  `json:"name"`
		URLKey            string  `json:"urlKey"`
		LogoURL           *string `json:"logoUrl"`
		UserCount         int64   `json:"userCount"`
		CreatedIssueCount int64   `json:"createdIssueCount"`
		RoadmapEnabled    bool    `json:"roadmapEnabled"`
		ReleasesEnabled   bool    `json:"releasesEnabled"`
		CustomersEnabled  bool    `json:"customersEnabled"`
		SAMLEnabled       bool    `json:"samlEnabled"`
		SCIMEnabled       bool    `json:"scimEnabled"`
	}
	if err := decodeField(data, "organization", &a); err != nil {
		return err
	}

	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.URLKey = types.StringValue(a.URLKey)
	m.LogoURL = types.StringPointerValue(a.LogoURL)
	m.UserCount = types.Int64Value(a.UserCount)
	m.CreatedIssueCount = types.Int64Value(a.CreatedIssueCount)
	m.RoadmapEnabled = types.BoolValue(a.RoadmapEnabled)
	m.ReleasesEnabled = types.BoolValue(a.ReleasesEnabled)
	m.CustomersEnabled = types.BoolValue(a.CustomersEnabled)
	m.SAMLEnabled = types.BoolValue(a.SAMLEnabled)
	m.SCIMEnabled = types.BoolValue(a.SCIMEnabled)
	return nil
}

func organizationDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Reads the Linear workspace the API key belongs to. Takes no arguments — the key " +
			"determines the workspace.\n\n" +
			"Use it to branch a configuration on what the workspace has enabled, e.g. only declaring " +
			"`linear_customer_tier` resources when `customers_enabled` is true.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the workspace.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the workspace.",
				Computed:            true,
			},
			"url_key": schema.StringAttribute{
				MarkdownDescription: "URL slug of the workspace — the `<key>` in `linear.app/<key>`.",
				Computed:            true,
			},
			"logo_url": schema.StringAttribute{
				MarkdownDescription: "URL of the workspace logo.",
				Computed:            true,
			},
			"user_count": schema.Int64Attribute{
				MarkdownDescription: "How many users the workspace has.",
				Computed:            true,
			},
			"created_issue_count": schema.Int64Attribute{
				MarkdownDescription: "How many issues have been created in the workspace.",
				Computed:            true,
			},
			"roadmap_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Initiatives are enabled. Linear renamed the feature to " +
					"Initiatives; its API still calls the field `roadmapEnabled`, the name it shipped under.",
				Computed: true,
			},
			"releases_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether releases are enabled — the prerequisite for `linear_release_pipeline`.",
				Computed:            true,
			},
			"customers_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Linear Customers is enabled — the prerequisite for " +
					"`linear_customer_status` and `linear_customer_tier`.",
				Computed: true,
			},
			"saml_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether SAML single sign-on is enabled.",
				Computed:            true,
			},
			"scim_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether SCIM user provisioning is enabled.",
				Computed:            true,
			},
		},
	}
}
