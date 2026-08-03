package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Triage responsibility says who picks up a team's incoming issues: either a
// rotation driven by a linear_time_schedule, or a fixed list of users.
var triageResponsibilityEntity = entity{
	name:   "triageResponsibility",
	fields: `id action team { id } timeSchedule { id } manualSelection { userIds assignmentIndex }`,
}

// NewTriageResponsibilityResource returns a new linear_triage_responsibility resource.
func NewTriageResponsibilityResource() resource.Resource {
	return &standardResource{
		entity:   triageResponsibilityEntity,
		typeName: "triage_responsibility",
		kind:     "triage responsibility",
		schema:   triageResponsibilitySchema,
		newModel: func() crudModel { return &triageResponsibilityModel{} },
	}
}

type triageResponsibilityAttributes struct {
	ID              string `json:"id"`
	Action          string `json:"action"`
	Team            *ref   `json:"team"`
	TimeSchedule    *ref   `json:"timeSchedule"`
	ManualSelection *struct {
		UserIDs         []string `json:"userIds"`
		AssignmentIndex *int64   `json:"assignmentIndex"`
	} `json:"manualSelection"`
}

type triageResponsibilityModel struct {
	ID             types.String `tfsdk:"id"`
	TeamID         types.String `tfsdk:"team_id"`
	Action         types.String `tfsdk:"action"`
	TimeScheduleID types.String `tfsdk:"time_schedule_id"`
	ManualUserIDs  types.List   `tfsdk:"manual_user_ids"`
}

func (m *triageResponsibilityModel) id() string { return m.ID.ValueString() }

func (m *triageResponsibilityModel) decode(ctx context.Context, raw json.RawMessage) error {
	var a triageResponsibilityAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Action = types.StringValue(a.Action)
	m.TeamID = refID(a.Team)
	m.TimeScheduleID = refID(a.TimeSchedule)

	if a.ManualSelection == nil {
		m.ManualUserIDs = types.ListNull(types.StringType)
		return nil
	}
	list, err := listOfStrings(ctx, a.ManualSelection.UserIDs)
	if err != nil {
		return err
	}
	m.ManualUserIDs = list
	return nil
}

func (m *triageResponsibilityModel) input(ctx context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"action": m.Action.ValueString()}
	// A null timeScheduleId on update is what switches a rota back to a manual
	// selection, so the null goes over explicitly.
	putString(in, "timeScheduleId", m.TimeScheduleID, forUpdate)

	switch {
	case !m.ManualUserIDs.IsNull() && !m.ManualUserIDs.IsUnknown():
		var ids []string
		_ = m.ManualUserIDs.ElementsAs(ctx, &ids, false)
		in["manualSelection"] = map[string]any{"userIds": orEmpty(ids)}
	case forUpdate:
		in["manualSelection"] = nil
	}

	if !forUpdate {
		in["teamId"] = m.TeamID.ValueString()
	}
	return in
}

func triageResponsibilitySchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages who is responsible for a Linear team's triage inbox.\n\n" +
			"Drive it either from a rota — set `time_schedule_id` to a `linear_time_schedule` — or from a fixed " +
			"list of people via `manual_user_ids`. The team needs `triage_enabled` for this to have any effect.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the triage responsibility.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team. Changing it replaces the resource — the update mutation " +
					"has no `teamId`.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"action": schema.StringAttribute{
				MarkdownDescription: "What happens to an incoming issue — `assign` puts it on the responsible " +
					"person, `notify` only tells them about it.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("assign", "notify"),
				},
			},
			"time_schedule_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the `linear_time_schedule` driving the rotation. Leave unset to use " +
					"`manual_user_ids` instead.",
				Optional: true,
			},
			"manual_user_ids": schema.ListAttribute{
				MarkdownDescription: "UUIDs of the users responsible, in rotation order. Leave unset when a " +
					"`time_schedule_id` drives the rotation.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}
