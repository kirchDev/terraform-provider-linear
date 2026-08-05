package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Views are the reason this provider exists — the community provider has none,
// and its pull request for them has been open since November 2023. What stalled
// it is the filter.
//
// IssueFilter has 122 top-level fields, arbitrarily nested, and Linear
// normalises filterData server-side before storing it. Typed HCL would freeze
// the provider against a schema that moves, and a plain string attribute drifts
// on every plan because the bytes coming back are not the bytes sent. So the
// filter goes through as raw JSON in a jsontypes.Normalized attribute, which
// compares semantically.
var customViewEntity = entity{
	name: "customView",
	fields: `id name description icon color shared modelName
		filterData projectFilterData initiativeFilterData feedItemFilterData
		team { id } owner { id }`,
}

// NewCustomViewResource returns a new linear_custom_view resource.
func NewCustomViewResource() resource.Resource {
	return &standardResource{
		entity:   customViewEntity,
		typeName: "custom_view",
		kind:     "custom view",
		schema:   customViewSchema,
		newModel: func() crudModel { return &customViewModel{} },
	}
}

type customViewAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	Color       *string `json:"color"`
	Shared      bool    `json:"shared"`
	ModelName   string  `json:"modelName"`

	FilterData           json.RawMessage `json:"filterData"`
	ProjectFilterData    json.RawMessage `json:"projectFilterData"`
	InitiativeFilterData json.RawMessage `json:"initiativeFilterData"`
	FeedItemFilterData   json.RawMessage `json:"feedItemFilterData"`

	Team  *ref `json:"team"`
	Owner *ref `json:"owner"`
}

type customViewModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Icon        types.String `tfsdk:"icon"`
	Color       types.String `tfsdk:"color"`
	Shared      types.Bool   `tfsdk:"shared"`
	ModelName   types.String `tfsdk:"model_name"`

	TeamID       types.String `tfsdk:"team_id"`
	ProjectID    types.String `tfsdk:"project_id"`
	InitiativeID types.String `tfsdk:"initiative_id"`
	OwnerID      types.String `tfsdk:"owner_id"`

	FilterJSON           jsontypes.Normalized `tfsdk:"filter_json"`
	ProjectFilterJSON    jsontypes.Normalized `tfsdk:"project_filter_json"`
	InitiativeFilterJSON jsontypes.Normalized `tfsdk:"initiative_filter_json"`
	FeedItemFilterJSON   jsontypes.Normalized `tfsdk:"feed_item_filter_json"`
}

func (m *customViewModel) id() string { return m.ID.ValueString() }

func (m *customViewModel) decode(_ context.Context, raw json.RawMessage) error {
	var a customViewAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.Icon = types.StringPointerValue(a.Icon)
	m.Color = types.StringPointerValue(a.Color)
	m.Shared = types.BoolValue(a.Shared)
	m.ModelName = types.StringValue(a.ModelName)
	m.TeamID = refID(a.Team)
	m.OwnerID = refID(a.Owner)

	m.FilterJSON = jsonAttr(a.FilterData)
	m.ProjectFilterJSON = jsonAttr(a.ProjectFilterData)
	m.InitiativeFilterJSON = jsonAttr(a.InitiativeFilterData)
	m.FeedItemFilterJSON = jsonAttr(a.FeedItemFilterData)

	// project_id and initiative_id are deliberately not touched: CustomView
	// exposes projects and initiatives as connections, not as the scalar ids the
	// input takes, so there is nothing to refresh them against. They keep
	// whatever the configuration declared.
	return nil
}

func (m *customViewModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}
	putString(in, "description", m.Description, false)
	putString(in, "icon", m.Icon, false)
	putString(in, "color", m.Color, false)
	putBool(in, "shared", m.Shared, false)
	// A null teamId on update is what widens a team view to the whole workspace,
	// so the null goes over explicitly — which is why team_id stays plain Optional
	// while description, icon and colour are Optional + Computed.
	putString(in, "teamId", m.TeamID, forUpdate)
	putString(in, "projectId", m.ProjectID, forUpdate)
	putString(in, "initiativeId", m.InitiativeID, forUpdate)
	putString(in, "ownerId", m.OwnerID, false)

	// The four filters are mutually exclusive — a validator enforces that — so at
	// most one of these ever lands in the input. Malformed JSON is impossible
	// here: jsontypes.Normalized rejects it at validate time.
	_ = putJSON(in, "filterData", m.FilterJSON, forUpdate)
	_ = putJSON(in, "projectFilterData", m.ProjectFilterJSON, forUpdate)
	_ = putJSON(in, "initiativeFilterData", m.InitiativeFilterJSON, forUpdate)
	_ = putJSON(in, "feedItemFilterData", m.FeedItemFilterJSON, forUpdate)
	return in
}

func customViewSchema() schema.Schema {
	// Exactly one filter may be set: which one decides the view's modelName, and
	// Linear rejects a view that mixes them.
	filterPaths := []path.Expression{
		path.MatchRoot("filter_json"),
		path.MatchRoot("project_filter_json"),
		path.MatchRoot("initiative_filter_json"),
		path.MatchRoot("feed_item_filter_json"),
	}
	filterAttr := func(desc string) schema.Attribute {
		return schema.StringAttribute{
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			CustomType:          jsontypes.NormalizedType{},
			Validators: []validator.String{
				stringvalidator.ConflictsWith(filterPaths...),
			},
			PlanModifiers: keepJSON(),
		}
	}

	return schema.Schema{
		MarkdownDescription: "Manages a Linear custom view — a saved, filtered list of issues, projects, " +
			"initiatives or feed items.\n\n" +
			"The filter goes through as a JSON object rather than typed HCL. Linear's `IssueFilter` alone has 122 " +
			"top-level fields and nests arbitrarily, and the server normalises what it stores, so a typed schema " +
			"would freeze against a moving API and a plain string would report drift on every plan. The " +
			"`*_json` attributes are compared semantically instead.\n\n" +
			"Set exactly one of `filter_json`, `project_filter_json`, `initiative_filter_json` or " +
			"`feed_item_filter_json` — which one decides what the view lists.\n\n" +
			"```terraform\n" +
			"resource \"linear_custom_view\" \"in_review\" {\n" +
			"  name    = \"In Review\"\n" +
			"  team_id = linear_team.eng.id\n" +
			"  shared  = true\n\n" +
			"  filter_json = jsonencode({\n" +
			"    state  = { type = { eq = \"started\" } }\n" +
			"    labels = { some = { name = { eq = \"ai\" } } }\n" +
			"  })\n" +
			"}\n" +
			"```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the view.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the view.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the view.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"icon": schema.StringAttribute{
				MarkdownDescription: "Icon of the view.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Colour of the view icon as a hex string.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},
			"shared": schema.BoolAttribute{
				MarkdownDescription: "Whether the view is visible to the whole workspace rather than only its owner.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"model_name": schema.StringAttribute{
				MarkdownDescription: "What the view lists, derived by Linear from whichever filter is set — " +
					"`Issue`, `Project`, `Initiative` or `FeedItem`.",
				Computed: true,
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the view is scoped to. Leave unset for a workspace-wide view.",
				Optional:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the project the view is scoped to. **Write-only**: Linear exposes a " +
					"view's projects as a connection rather than as this id, so drift in this attribute cannot " +
					"be detected.",
				Optional: true,
			},
			"initiative_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the initiative the view is scoped to. **Write-only**, for the same " +
					"reason as `project_id`.",
				Optional: true,
			},
			"owner_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the user who owns the view. Defaults to whoever the API key belongs to.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepString(),
			},

			"filter_json": filterAttr("Issue filter as a JSON object, e.g. " +
				"`jsonencode({ state = { type = { eq = \"started\" } } })`. Compared semantically, so Linear's " +
				"server-side normalisation is not reported as drift."),
			"project_filter_json":    filterAttr("Project filter as a JSON object. Makes this a project view."),
			"initiative_filter_json": filterAttr("Initiative filter as a JSON object. Makes this an initiative view."),
			"feed_item_filter_json":  filterAttr("Feed item filter as a JSON object. Makes this a feed view."),
		},
	}
}
