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

// The selection is deliberately narrower than the resource's: a data source
// exists to reference a team from elsewhere in the configuration, so identity
// and the handful of settings worth branching on are enough.
const teamDataSourceFields = `id name key description icon color private timezone
	cyclesEnabled triageEnabled parent { id }`

// NewTeamDataSource returns a new linear_team data source.
func NewTeamDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "team",
		kind:     "team",
		schema:   teamDataSourceSchema,
		newModel: func() any { return &teamDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readTeam(ctx, c, model.(*teamDataSourceModel))
		},
	}
}

// NewTeamsDataSource returns a new linear_teams data source.
func NewTeamsDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "teams",
		kind:     "teams",
		schema:   teamsDataSourceSchema,
		newModel: func() any { return &teamsDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readTeams(ctx, c, model.(*teamsDataSourceModel))
		},
	}
}

type teamDataSourceAttributes struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Key           string  `json:"key"`
	Description   *string `json:"description"`
	Icon          *string `json:"icon"`
	Color         *string `json:"color"`
	Private       bool    `json:"private"`
	Timezone      string  `json:"timezone"`
	CyclesEnabled bool    `json:"cyclesEnabled"`
	TriageEnabled bool    `json:"triageEnabled"`
	Parent        *ref    `json:"parent"`
}

type teamDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Key           types.String `tfsdk:"key"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Icon          types.String `tfsdk:"icon"`
	Color         types.String `tfsdk:"color"`
	Private       types.Bool   `tfsdk:"private"`
	Timezone      types.String `tfsdk:"timezone"`
	CyclesEnabled types.Bool   `tfsdk:"cycles_enabled"`
	TriageEnabled types.Bool   `tfsdk:"triage_enabled"`
	ParentID      types.String `tfsdk:"parent_id"`
}

func (m *teamDataSourceModel) fill(a *teamDataSourceAttributes) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Key = types.StringValue(a.Key)
	m.Description = types.StringPointerValue(a.Description)
	m.Icon = types.StringPointerValue(a.Icon)
	m.Color = types.StringPointerValue(a.Color)
	m.Private = types.BoolValue(a.Private)
	m.Timezone = types.StringValue(a.Timezone)
	m.CyclesEnabled = types.BoolValue(a.CyclesEnabled)
	m.TriageEnabled = types.BoolValue(a.TriageEnabled)
	m.ParentID = refID(a.Parent)
}

func readTeam(ctx context.Context, c *client.Client, m *teamDataSourceModel) error {
	// By id when given, otherwise by key — the identifier prefix humans use.
	if !m.ID.IsNull() && m.ID.ValueString() != "" {
		var raw json.RawMessage
		if err := (entity{name: "team", fields: teamDataSourceFields}).read(ctx, c, m.ID.ValueString(), &raw); err != nil {
			return err
		}
		var a teamDataSourceAttributes
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		m.fill(&a)
		return nil
	}

	if m.Key.IsNull() || m.Key.ValueString() == "" {
		return fmt.Errorf("set either id or key")
	}

	var teams []teamDataSourceAttributes
	err := connection(ctx, c, "teams", "$filter: TeamFilter", "filter: $filter", teamDataSourceFields,
		map[string]any{"filter": map[string]any{"key": map[string]any{"eq": m.Key.ValueString()}}}, &teams)
	if err != nil {
		return err
	}
	if len(teams) == 0 {
		return fmt.Errorf("no team with key %q", m.Key.ValueString())
	}
	m.fill(&teams[0])
	return nil
}

type teamsDataSourceModel struct {
	Teams []teamDataSourceModel `tfsdk:"teams"`
}

func readTeams(ctx context.Context, c *client.Client, m *teamsDataSourceModel) error {
	var teams []teamDataSourceAttributes
	if err := connection(ctx, c, "teams", "", "", teamDataSourceFields, nil, &teams); err != nil {
		return err
	}
	m.Teams = make([]teamDataSourceModel, 0, len(teams))
	for i := range teams {
		var one teamDataSourceModel
		one.fill(&teams[i])
		m.Teams = append(m.Teams, one)
	}
	return nil
}

func teamDataSourceAttributeSchema(computedOnly bool) map[string]schema.Attribute {
	idAttr := schema.StringAttribute{
		MarkdownDescription: "UUID of the team.",
		Computed:            true,
	}
	keyAttr := schema.StringAttribute{
		MarkdownDescription: "Team key — the issue identifier prefix, e.g. `ENG`.",
		Computed:            true,
	}
	if !computedOnly {
		idAttr.Optional = true
		idAttr.MarkdownDescription = "UUID of the team. Set either this or `key`."
		keyAttr.Optional = true
		keyAttr.MarkdownDescription = "Team key — the issue identifier prefix, e.g. `ENG`. Set either this or `id`."
	}

	return map[string]schema.Attribute{
		"id":  idAttr,
		"key": keyAttr,
		"name": schema.StringAttribute{
			MarkdownDescription: "Name of the team.",
			Computed:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "Description of the team.",
			Computed:            true,
		},
		"icon": schema.StringAttribute{
			MarkdownDescription: "Icon of the team.",
			Computed:            true,
		},
		"color": schema.StringAttribute{
			MarkdownDescription: "Colour of the team as a hex string.",
			Computed:            true,
		},
		"private": schema.BoolAttribute{
			MarkdownDescription: "Whether the team is private.",
			Computed:            true,
		},
		"timezone": schema.StringAttribute{
			MarkdownDescription: "Timezone the team's cycles and SLAs are computed in.",
			Computed:            true,
		},
		"cycles_enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether Linear generates cycles for the team.",
			Computed:            true,
		},
		"triage_enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether the team has a triage inbox.",
			Computed:            true,
		},
		"parent_id": schema.StringAttribute{
			MarkdownDescription: "UUID of the parent team, if the team nests under one.",
			Computed:            true,
		},
	}
}

func teamDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Looks up a Linear team by UUID or by team key — useful for referencing a team this " +
			"configuration does not manage.",
		Attributes: teamDataSourceAttributeSchema(false),
	}
}

func teamsDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Lists every Linear team in the workspace.",
		Attributes: map[string]schema.Attribute{
			"teams": schema.ListNestedAttribute{
				MarkdownDescription: "The teams, as Linear returns them.",
				Computed:            true,
				NestedObject:        schema.NestedAttributeObject{Attributes: teamDataSourceAttributeSchema(true)},
			},
		},
	}
}
