package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A time schedule is the on-call rota a triage responsibility rotates through.
// Its entries are small and symmetric between input and output, so they are
// modelled as typed HCL rather than as a JSON blob.
var timeScheduleEntity = entity{
	name:   "timeSchedule",
	fields: `id name externalId externalUrl entries { startsAt endsAt userId userEmail }`,
}

// NewTimeScheduleResource returns a new linear_time_schedule resource.
func NewTimeScheduleResource() resource.Resource {
	return &standardResource{
		entity:   timeScheduleEntity,
		typeName: "time_schedule",
		kind:     "time schedule",
		schema:   timeScheduleSchema,
		newModel: func() crudModel { return &timeScheduleModel{} },
	}
}

type timeScheduleEntryAttributes struct {
	StartsAt  string  `json:"startsAt"`
	EndsAt    string  `json:"endsAt"`
	UserID    *string `json:"userId"`
	UserEmail *string `json:"userEmail"`
}

type timeScheduleAttributes struct {
	ID          string                        `json:"id"`
	Name        string                        `json:"name"`
	ExternalID  *string                       `json:"externalId"`
	ExternalURL *string                       `json:"externalUrl"`
	Entries     []timeScheduleEntryAttributes `json:"entries"`
}

type timeScheduleEntryModel struct {
	StartsAt  types.String `tfsdk:"starts_at"`
	EndsAt    types.String `tfsdk:"ends_at"`
	UserID    types.String `tfsdk:"user_id"`
	UserEmail types.String `tfsdk:"user_email"`
}

type timeScheduleModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ExternalID  types.String `tfsdk:"external_id"`
	ExternalURL types.String `tfsdk:"external_url"`
	Entries     types.List   `tfsdk:"entries"`
}

func timeScheduleEntryType() attr.Type {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"starts_at":  types.StringType,
		"ends_at":    types.StringType,
		"user_id":    types.StringType,
		"user_email": types.StringType,
	}}
}

func (m *timeScheduleModel) id() string { return m.ID.ValueString() }

func (m *timeScheduleModel) decode(ctx context.Context, raw json.RawMessage) error {
	var a timeScheduleAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.ExternalID = types.StringPointerValue(a.ExternalID)
	m.ExternalURL = types.StringPointerValue(a.ExternalURL)

	entries := make([]timeScheduleEntryModel, 0, len(a.Entries))
	for _, e := range a.Entries {
		entries = append(entries, timeScheduleEntryModel{
			StartsAt:  types.StringValue(e.StartsAt),
			EndsAt:    types.StringValue(e.EndsAt),
			UserID:    types.StringPointerValue(e.UserID),
			UserEmail: types.StringPointerValue(e.UserEmail),
		})
	}
	list, d := types.ListValueFrom(ctx, timeScheduleEntryType(), entries)
	if d.HasError() {
		return errBuildingList("entries")
	}
	m.Entries = list
	return nil
}

func (m *timeScheduleModel) input(ctx context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}
	putString(in, "externalId", m.ExternalID, false)
	putString(in, "externalUrl", m.ExternalURL, false)

	// entries is required on create and replaces the whole rota on update, so it
	// always goes over in full.
	var entries []timeScheduleEntryModel
	if !m.Entries.IsNull() && !m.Entries.IsUnknown() {
		_ = m.Entries.ElementsAs(ctx, &entries, false)
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		entry := map[string]any{
			"startsAt": e.StartsAt.ValueString(),
			"endsAt":   e.EndsAt.ValueString(),
		}
		putString(entry, "userId", e.UserID, false)
		putString(entry, "userEmail", e.UserEmail, false)
		out = append(out, entry)
	}
	in["entries"] = out
	return in
}

func timeScheduleSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a Linear time schedule — the on-call rota a " +
			"`linear_triage_responsibility` rotates through.\n\n" +
			"Entries are absolute time windows, not a recurrence rule: Linear stores who is on call between two " +
			"instants. A rota that repeats has to be generated, which is what the external schedule integrations " +
			"do — point `external_id` and `external_url` at one to keep the provenance visible.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the schedule.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the schedule.",
				Required:            true,
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the schedule in the external system it mirrors.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"external_url": schema.StringAttribute{
				MarkdownDescription: "URL of the schedule in the external system it mirrors.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"entries": schema.ListNestedAttribute{
				MarkdownDescription: "Shifts making up the rota. Sending this replaces every entry — there is no " +
					"partial update.",
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"starts_at": schema.StringAttribute{
							MarkdownDescription: "When the shift starts, as an RFC 3339 timestamp.",
							Required:            true,
						},
						"ends_at": schema.StringAttribute{
							MarkdownDescription: "When the shift ends, as an RFC 3339 timestamp.",
							Required:            true,
						},
						"user_id": schema.StringAttribute{
							MarkdownDescription: "UUID of the user on call. Set either this or `user_email`.",
							Optional:            true,
						},
						"user_email": schema.StringAttribute{
							MarkdownDescription: "Email of the user on call — useful when the person is not yet a " +
								"workspace member. Set either this or `user_id`.",
							Optional: true,
						},
					},
				},
			},
		},
	}
}
