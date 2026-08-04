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

// Customer statuses are the lifecycle stages a customer moves through — the
// customer-side counterpart of workflow states. They only exist once Linear
// Customers is enabled for the workspace (`customers_enabled` on
// linear_workspace_settings).
var customerStatusEntity = entity{
	name:   "customerStatus",
	fields: `id name displayName description color position`,
}

// NewCustomerStatusResource returns a new linear_customer_status resource.
func NewCustomerStatusResource() resource.Resource {
	return &standardResource{
		entity:   customerStatusEntity,
		typeName: "customer_status",
		kind:     "customer status",
		schema:   customerStatusSchema,
		newModel: func() crudModel { return &customerStatusModel{} },
	}
}

type customerStatusAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	DisplayName string  `json:"displayName"`
	Description *string `json:"description"`
	Color       string  `json:"color"`
	Position    float64 `json:"position"`
}

type customerStatusModel struct {
	ID          types.String  `tfsdk:"id"`
	Name        types.String  `tfsdk:"name"`
	DisplayName types.String  `tfsdk:"display_name"`
	Description types.String  `tfsdk:"description"`
	Color       types.String  `tfsdk:"color"`
	Position    types.Float64 `tfsdk:"position"`
}

func (m *customerStatusModel) id() string { return m.ID.ValueString() }

func (m *customerStatusModel) decode(_ context.Context, raw json.RawMessage) error {
	var a customerStatusAttributes
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

func (m *customerStatusModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"color": m.Color.ValueString()}
	putString(in, "name", m.Name, false)
	putString(in, "displayName", m.DisplayName, false)
	putString(in, "description", m.Description, false)
	putFloat(in, "position", m.Position, false)
	return in
}

func customerStatusSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a customer status in Linear — one stage of the workspace's customer " +
			"lifecycle.\n\n" +
			"Requires Linear Customers to be enabled for the workspace; see `customers_enabled` on " +
			"`linear_workspace_settings`. Statuses are workspace-wide, not per team.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the customer status.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Internal name of the status. Set at least one of `name` and `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Name shown in the Linear UI. Set at least one of `name` and `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of what the status represents.",
				Optional:            true,
				Computed:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the status indicator as a hex string, e.g. `#26b5ce`.",
				Required:            true,
			},
			"position": schema.Float64Attribute{
				MarkdownDescription: "Sort position within the customer lifecycle. Linear appends the status at " +
					"the end when unset.",
				Optional: true,
				Computed: true,
			},
		},
	}
}
