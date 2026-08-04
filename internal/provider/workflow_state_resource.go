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

// Linear soft-deletes workflow states: there is no workflowStateDelete, only
// workflowStateArchive.
var workflowStateEntity = entity{
	name:       "workflowState",
	fields:     `id name color description type position team { id }`,
	deleteVerb: "Archive",
}

// NewWorkflowStateResource returns a new linear_workflow_state resource.
func NewWorkflowStateResource() resource.Resource {
	return &standardResource{
		entity:    workflowStateEntity,
		typeName:  "workflow_state",
		kind:      "workflow state",
		schema:    workflowStateSchema,
		newModel:  func() crudModel { return &workflowStateModel{} },
		deleteMsg: "Unable to archive Linear workflow state",
	}
}

type workflowStateAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
	Type        string  `json:"type"`
	Position    float64 `json:"position"`
	Team        *ref    `json:"team"`
}

type workflowStateModel struct {
	ID          types.String  `tfsdk:"id"`
	TeamID      types.String  `tfsdk:"team_id"`
	Name        types.String  `tfsdk:"name"`
	Color       types.String  `tfsdk:"color"`
	Description types.String  `tfsdk:"description"`
	Type        types.String  `tfsdk:"type"`
	Position    types.Float64 `tfsdk:"position"`
}

func (m *workflowStateModel) id() string { return m.ID.ValueString() }

func (m *workflowStateModel) decode(_ context.Context, raw json.RawMessage) error {
	var a workflowStateAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Color = types.StringValue(a.Color)
	m.Description = types.StringPointerValue(a.Description)
	m.Type = types.StringValue(a.Type)
	m.Position = types.Float64Value(a.Position)
	m.TeamID = refID(a.Team)
	return nil
}

func (m *workflowStateModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{
		"name":  m.Name.ValueString(),
		"color": m.Color.ValueString(),
	}
	putString(in, "description", m.Description, false)
	putFloat(in, "position", m.Position, false)
	// Neither type nor teamId exists on WorkflowStateUpdateInput — both are
	// RequiresReplace in the schema for exactly that reason.
	if !forUpdate {
		in["type"] = m.Type.ValueString()
		in["teamId"] = m.TeamID.ValueString()
	}
	return in
}

func workflowStateSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a workflow state (issue status) of a Linear team.\n\n" +
			"Destroying the resource archives the state — Linear has no hard delete for workflow states. A state " +
			"that still holds issues cannot be archived until they are moved.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the workflow state.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the state belongs to. Changing it replaces the state — " +
					"`workflowStateUpdate` has no `teamId`.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the state, e.g. `In Review`.",
				Required:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the state as a hex string, e.g. `#f2c94c`.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the state.",
				Optional:            true,
				Computed:            true,
			},
			// Deliberately a plain string, not an enum: Linear types this field as
			// String and `duplicate` is a real value the API returns. Modelling it as
			// an enum is what leaves every team's Duplicate state unmanageable in the
			// community provider.
			"type": schema.StringAttribute{
				MarkdownDescription: "Category the state belongs to — one of `triage`, `backlog`, `unstarted`, " +
					"`started`, `completed`, `canceled`, `duplicate`. Changing it replaces the state, since " +
					"`workflowStateUpdate` has no `type`.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"position": schema.Float64Attribute{
				MarkdownDescription: "Sort position of the state within its category. Linear assigns one when unset.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}
