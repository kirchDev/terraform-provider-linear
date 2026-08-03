package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var releasePipelineEntity = entity{
	name: "releasePipeline",
	fields: `id name slugId type isProduction includePathPatterns
		autoGenerateReleaseNotesOnCompletion teams { nodes { id } }`,
	deleteVerb: "Archive",
}

// NewReleasePipelineResource returns a new linear_release_pipeline resource.
func NewReleasePipelineResource() resource.Resource {
	return &standardResource{
		entity:    releasePipelineEntity,
		typeName:  "release_pipeline",
		kind:      "release pipeline",
		schema:    releasePipelineSchema,
		newModel:  func() crudModel { return &releasePipelineModel{} },
		deleteMsg: "Unable to archive Linear release pipeline",
	}
}

type releasePipelineAttributes struct {
	ID                                   string   `json:"id"`
	Name                                 string   `json:"name"`
	SlugID                               string   `json:"slugId"`
	Type                                 string   `json:"type"`
	IsProduction                         bool     `json:"isProduction"`
	IncludePathPatterns                  []string `json:"includePathPatterns"`
	AutoGenerateReleaseNotesOnCompletion bool     `json:"autoGenerateReleaseNotesOnCompletion"`
	Teams                                struct {
		Nodes []ref `json:"nodes"`
	} `json:"teams"`
}

type releasePipelineModel struct {
	ID                                   types.String `tfsdk:"id"`
	Name                                 types.String `tfsdk:"name"`
	SlugID                               types.String `tfsdk:"slug_id"`
	Type                                 types.String `tfsdk:"type"`
	IsProduction                         types.Bool   `tfsdk:"is_production"`
	IncludePathPatterns                  types.List   `tfsdk:"include_path_patterns"`
	AutoGenerateReleaseNotesOnCompletion types.Bool   `tfsdk:"auto_generate_release_notes_on_completion"`
	TeamIDs                              types.Set    `tfsdk:"team_ids"`
}

func (m *releasePipelineModel) id() string { return m.ID.ValueString() }

func (m *releasePipelineModel) decode(ctx context.Context, raw json.RawMessage) error {
	var a releasePipelineAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.SlugID = types.StringValue(a.SlugID)
	m.Type = types.StringValue(a.Type)
	m.IsProduction = types.BoolValue(a.IsProduction)
	m.AutoGenerateReleaseNotesOnCompletion = types.BoolValue(a.AutoGenerateReleaseNotesOnCompletion)

	patterns, err := listOfStrings(ctx, a.IncludePathPatterns)
	if err != nil {
		return err
	}
	m.IncludePathPatterns = patterns

	ids := make([]string, 0, len(a.Teams.Nodes))
	for _, t := range a.Teams.Nodes {
		ids = append(ids, t.ID)
	}
	teams, err := setOfStrings(ctx, ids)
	if err != nil {
		return err
	}
	m.TeamIDs = teams
	return nil
}

func (m *releasePipelineModel) input(ctx context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}
	putString(in, "slugId", m.SlugID, false)
	putString(in, "type", m.Type, false)
	putBool(in, "isProduction", m.IsProduction, false)
	putBool(in, "autoGenerateReleaseNotesOnCompletion", m.AutoGenerateReleaseNotesOnCompletion, false)
	_ = putStringList(ctx, in, "includePathPatterns", m.IncludePathPatterns, false)
	_ = putStringSet(ctx, in, "teamIds", m.TeamIDs, false)
	return in
}

func releasePipelineSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a Linear release pipeline — how a set of teams ships, and which paths in " +
			"the repository count towards a release.\n\n" +
			"Destroying the resource archives the pipeline.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the pipeline.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the pipeline.",
				Required:            true,
			},
			"slug_id": schema.StringAttribute{
				MarkdownDescription: "URL slug of the pipeline. Linear derives one from the name when unset.",
				Optional:            true,
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "How the pipeline releases — `continuous` ships on merge, `scheduled` ships " +
					"on a cadence.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf("continuous", "scheduled"),
				},
			},
			"is_production": schema.BoolAttribute{
				MarkdownDescription: "Whether this pipeline ships to production.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"include_path_patterns": schema.ListAttribute{
				MarkdownDescription: "Repository path patterns a change has to touch to belong to this pipeline, " +
					"e.g. `[\"apps/web/**\"]`. An empty list includes everything.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"auto_generate_release_notes_on_completion": schema.BoolAttribute{
				MarkdownDescription: "Whether Linear writes release notes itself when a release completes.",
				Optional:            true,
				Computed:            true,
			},
			"team_ids": schema.SetAttribute{
				MarkdownDescription: "UUIDs of the teams shipping through this pipeline.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}
