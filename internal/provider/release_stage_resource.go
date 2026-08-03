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

var releaseStageEntity = entity{
	name:       "releaseStage",
	fields:     `id name color type position frozen pipeline { id }`,
	deleteVerb: "Archive",
}

// NewReleaseStageResource returns a new linear_release_stage resource.
func NewReleaseStageResource() resource.Resource {
	return &standardResource{
		entity:    releaseStageEntity,
		typeName:  "release_stage",
		kind:      "release stage",
		schema:    releaseStageSchema,
		newModel:  func() crudModel { return &releaseStageModel{} },
		deleteMsg: "Unable to archive Linear release stage",
	}
}

type releaseStageAttributes struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	Type     string  `json:"type"`
	Position float64 `json:"position"`
	Frozen   bool    `json:"frozen"`
	Pipeline *ref    `json:"pipeline"`
}

type releaseStageModel struct {
	ID         types.String  `tfsdk:"id"`
	PipelineID types.String  `tfsdk:"pipeline_id"`
	Name       types.String  `tfsdk:"name"`
	Color      types.String  `tfsdk:"color"`
	Type       types.String  `tfsdk:"type"`
	Position   types.Float64 `tfsdk:"position"`
	Frozen     types.Bool    `tfsdk:"frozen"`
}

func (m *releaseStageModel) id() string { return m.ID.ValueString() }

func (m *releaseStageModel) decode(_ context.Context, raw json.RawMessage) error {
	var a releaseStageAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Color = types.StringValue(a.Color)
	m.Type = types.StringValue(a.Type)
	m.Position = types.Float64Value(a.Position)
	m.Frozen = types.BoolValue(a.Frozen)
	m.PipelineID = refID(a.Pipeline)
	return nil
}

func (m *releaseStageModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{
		"name":     m.Name.ValueString(),
		"color":    m.Color.ValueString(),
		"position": m.Position.ValueFloat64(),
	}
	putBool(in, "frozen", m.Frozen, false)
	// Neither type nor pipelineId exists on ReleaseStageUpdateInput.
	if !forUpdate {
		in["type"] = m.Type.ValueString()
		in["pipelineId"] = m.PipelineID.ValueString()
	}
	return in
}

func releaseStageSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a stage of a Linear release pipeline — one step a release passes through.\n\n" +
			"Destroying the resource archives the stage.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the stage.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"pipeline_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the `linear_release_pipeline` the stage belongs to. Changing it " +
					"replaces the stage.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the stage, e.g. `Canary`.",
				Required:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the stage indicator as a hex string.",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Category the stage belongs to — one of `planned`, `started`, `completed`, " +
					"`canceled`. Changing it replaces the stage.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("planned", "started", "completed", "canceled"),
				},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"position": schema.Float64Attribute{
				MarkdownDescription: "Sort position of the stage within the pipeline.",
				Required:            true,
			},
			"frozen": schema.BoolAttribute{
				MarkdownDescription: "Whether the stage is frozen — no release may move into it.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}
