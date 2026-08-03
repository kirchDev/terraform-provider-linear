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

const workflowStateDataSourceFields = `id name color description type position team { id }`

// NewWorkflowStateDataSource returns a new linear_workflow_state data source.
func NewWorkflowStateDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "workflow_state",
		kind:     "workflow state",
		schema:   workflowStateDataSourceSchema,
		newModel: func() any { return &workflowStateDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readWorkflowState(ctx, c, model.(*workflowStateDataSourceModel))
		},
	}
}

// NewWorkflowStatesDataSource returns a new linear_workflow_states data source.
func NewWorkflowStatesDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "workflow_states",
		kind:     "workflow states",
		schema:   workflowStatesDataSourceSchema,
		newModel: func() any { return &workflowStatesDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readWorkflowStates(ctx, c, model.(*workflowStatesDataSourceModel))
		},
	}
}

type workflowStateDataSourceModel struct {
	ID          types.String  `tfsdk:"id"`
	TeamID      types.String  `tfsdk:"team_id"`
	Name        types.String  `tfsdk:"name"`
	Color       types.String  `tfsdk:"color"`
	Description types.String  `tfsdk:"description"`
	Type        types.String  `tfsdk:"type"`
	Position    types.Float64 `tfsdk:"position"`
}

func (m *workflowStateDataSourceModel) fill(a *workflowStateAttributes) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Color = types.StringValue(a.Color)
	m.Description = types.StringPointerValue(a.Description)
	m.Type = types.StringValue(a.Type)
	m.Position = types.Float64Value(a.Position)
	m.TeamID = refID(a.Team)
}

func readWorkflowState(ctx context.Context, c *client.Client, m *workflowStateDataSourceModel) error {
	if !m.ID.IsNull() && m.ID.ValueString() != "" {
		var raw json.RawMessage
		e := entity{name: "workflowState", fields: workflowStateDataSourceFields}
		if err := e.read(ctx, c, m.ID.ValueString(), &raw); err != nil {
			return err
		}
		var a workflowStateAttributes
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		m.fill(&a)
		return nil
	}

	// A state name is unique per team, not per workspace, so a lookup by name
	// needs the team to disambiguate.
	if m.TeamID.IsNull() || m.Name.IsNull() {
		return fmt.Errorf("set either id, or both team_id and name")
	}

	filter := map[string]any{
		"team": map[string]any{"id": map[string]any{"eq": m.TeamID.ValueString()}},
		"name": map[string]any{"eq": m.Name.ValueString()},
	}
	var states []workflowStateAttributes
	err := connection(ctx, c, "workflowStates", "$filter: WorkflowStateFilter", "filter: $filter",
		workflowStateDataSourceFields, map[string]any{"filter": filter}, &states)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return fmt.Errorf("no workflow state named %q in team %s", m.Name.ValueString(), m.TeamID.ValueString())
	}
	m.fill(&states[0])
	return nil
}

type workflowStatesDataSourceModel struct {
	TeamID types.String                   `tfsdk:"team_id"`
	States []workflowStateDataSourceModel `tfsdk:"workflow_states"`
}

func readWorkflowStates(ctx context.Context, c *client.Client, m *workflowStatesDataSourceModel) error {
	params, args := "", ""
	var vars map[string]any
	if !m.TeamID.IsNull() && m.TeamID.ValueString() != "" {
		params, args = "$filter: WorkflowStateFilter", "filter: $filter"
		vars = map[string]any{"filter": map[string]any{
			"team": map[string]any{"id": map[string]any{"eq": m.TeamID.ValueString()}},
		}}
	}

	var states []workflowStateAttributes
	if err := connection(ctx, c, "workflowStates", params, args, workflowStateDataSourceFields, vars, &states); err != nil {
		return err
	}
	m.States = make([]workflowStateDataSourceModel, 0, len(states))
	for i := range states {
		var one workflowStateDataSourceModel
		one.fill(&states[i])
		m.States = append(m.States, one)
	}
	return nil
}

func workflowStateDataSourceAttributeSchema(computedOnly bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "UUID of the workflow state.",
			Computed:            true,
		},
		"team_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the team the state belongs to.",
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Name of the state.",
			Computed:            true,
		},
		"color": schema.StringAttribute{
			MarkdownDescription: "Colour of the state as a hex string.",
			Computed:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "Description of the state.",
			Computed:            true,
		},
		"type": schema.StringAttribute{
			MarkdownDescription: "Category the state belongs to — `triage`, `backlog`, `unstarted`, `started`, " +
				"`completed`, `canceled` or `duplicate`.",
			Computed: true,
		},
		"position": schema.Float64Attribute{
			MarkdownDescription: "Sort position of the state within its category.",
			Computed:            true,
		},
	}
	if !computedOnly {
		attrs["id"] = schema.StringAttribute{
			MarkdownDescription: "UUID of the state. Set either this, or `team_id` together with `name`.",
			Optional:            true,
			Computed:            true,
		}
		attrs["team_id"] = schema.StringAttribute{
			MarkdownDescription: "UUID of the team. Set together with `name` when looking a state up by name — " +
				"state names are unique per team, not per workspace.",
			Optional: true,
			Computed: true,
		}
		attrs["name"] = schema.StringAttribute{
			MarkdownDescription: "Name of the state. Set together with `team_id`.",
			Optional:            true,
			Computed:            true,
		}
	}
	return attrs
}

func workflowStateDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Looks up a workflow state of a Linear team, by UUID or by team and name.\n\n" +
			"Useful for pointing a `linear_git_automation_state` at a state this configuration does not manage.",
		Attributes: workflowStateDataSourceAttributeSchema(false),
	}
}

func workflowStatesDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Lists workflow states, either of one team or of the whole workspace.",
		Attributes: map[string]schema.Attribute{
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team to list states for. Leave unset to list every state in " +
					"the workspace.",
				Optional: true,
			},
			"workflow_states": schema.ListNestedAttribute{
				MarkdownDescription: "The workflow states, as Linear returns them.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: workflowStateDataSourceAttributeSchema(true),
				},
			},
		},
	}
}
