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

// Customer tiers segment customers by importance — the enterprise/pro/free
// banding requests get weighed against. Like statuses they are workspace-wide
// and need Linear Customers enabled.
var customerTierEntity = entity{
	name:   "customerTier",
	fields: `id name displayName description color position`,
}

// NewCustomerTierResource returns a new linear_customer_tier resource.
func NewCustomerTierResource() resource.Resource {
	return &standardResource{
		entity:   customerTierEntity,
		typeName: "customer_tier",
		kind:     "customer tier",
		schema:   customerTierSchema,
		newModel: func() crudModel { return &customerTierModel{} },
	}
}

type customerTierAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	DisplayName string  `json:"displayName"`
	Description *string `json:"description"`
	Color       string  `json:"color"`
	Position    float64 `json:"position"`
}

type customerTierModel struct {
	ID          types.String  `tfsdk:"id"`
	Name        types.String  `tfsdk:"name"`
	DisplayName types.String  `tfsdk:"display_name"`
	Description types.String  `tfsdk:"description"`
	Color       types.String  `tfsdk:"color"`
	Position    types.Float64 `tfsdk:"position"`
}

func (m *customerTierModel) id() string { return m.ID.ValueString() }

func (m *customerTierModel) decode(_ context.Context, raw json.RawMessage) error {
	var a customerTierAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.DisplayName = types.StringValue(a.DisplayName)
	m.Description = types.StringPointerValue(a.Description)
	m.Color = types.StringValue(a.Color)
	m.Position = types.Float64Value(a.Position)
	return nil
}

func (m *customerTierModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"color": m.Color.ValueString()}
	putString(in, "name", m.Name, false)
	putString(in, "displayName", m.DisplayName, false)
	putString(in, "description", m.Description, false)
	putFloat(in, "position", m.Position, false)
	return in
}

func customerTierSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a customer tier in Linear — the band a customer is segmented into, which " +
			"customer requests are weighed by.\n\n" +
			"Requires Linear Customers to be enabled for the workspace; see `customers_enabled` on " +
			"`linear_workspace_settings`. Tiers are workspace-wide, not per team.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the customer tier.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			// Neither carries keepString(), deliberately: each fills in for the
			// other, so changing one leaves the other genuinely unknown until
			// Linear has answered. See plan.go and derivedFromAnotherAttribute.
			"name": schema.StringAttribute{
				MarkdownDescription: "Internal name of the tier. Unique within the workspace. Set at least one of " +
					"`name` and `display_name`.",
				Optional: true,
				Computed: true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Name shown in the Linear UI. Set at least one of `name` and `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of what the tier represents.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the tier indicator as a hex string, e.g. `#f2c94c`.",
				Required:            true,
			},
			"position": schema.Float64Attribute{
				MarkdownDescription: "Sort position within the workspace's tier ordering. Linear appends the tier " +
					"at the end when unset.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepFloat(),
			},
		},
	}
}
