package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// View preferences are how a custom view remembers its layout, grouping and
// ordering. Two things about the API shape this resource:
//
//  1. There is no top-level viewPreferences(id:) query. The only way back to
//     them is through the view — customView(id:).organizationViewPreferences —
//     so this resource is anchored to a custom view and takes custom_view_id as
//     a required, replace-forcing attribute. Linear can attach preferences to a
//     team, project or label as well; those have no read path, so they are out
//     of scope rather than half-supported.
//
//  2. preferences goes in as a JSONObject but comes back as ViewPreferencesValues,
//     a typed object with over 250 fields. Selecting all of them would flood
//     state with nulls, so the read selects exactly the keys the configuration
//     sets. Drift is therefore detected for the preferences under management and
//     for no others — which is the useful behaviour anyway.

var (
	_ resource.Resource                = (*viewPreferencesResource)(nil)
	_ resource.ResourceWithConfigure   = (*viewPreferencesResource)(nil)
	_ resource.ResourceWithImportState = (*viewPreferencesResource)(nil)
)

// NewViewPreferencesResource returns a new linear_view_preferences resource.
func NewViewPreferencesResource() resource.Resource {
	return &viewPreferencesResource{}
}

type viewPreferencesResource struct {
	client *client.Client
}

type viewPreferencesModel struct {
	ID              types.String         `tfsdk:"id"`
	CustomViewID    types.String         `tfsdk:"custom_view_id"`
	ViewType        types.String         `tfsdk:"view_type"`
	Type            types.String         `tfsdk:"type"`
	PreferencesJSON jsontypes.Normalized `tfsdk:"preferences_json"`
}

func (r *viewPreferencesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_view_preferences"
}

func (r *viewPreferencesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the workspace-wide display preferences of a Linear custom view — its " +
			"layout, grouping, ordering and which columns are shown.\n\n" +
			"The preferences object has over 250 fields whose set follows what Linear's UI offers, so it goes " +
			"through as JSON rather than typed HCL. Only the keys the configuration sets are read back, so drift " +
			"is reported for the preferences under management and no others.\n\n" +
			"```terraform\n" +
			"resource \"linear_view_preferences\" \"in_review\" {\n" +
			"  custom_view_id = linear_custom_view.in_review.id\n" +
			"  view_type      = \"customView\"\n\n" +
			"  preferences_json = jsonencode({\n" +
			"    layout       = \"board\"\n" +
			"    issueGrouping = \"assignee\"\n" +
			"    viewOrdering = \"priority\"\n" +
			"  })\n" +
			"}\n" +
			"```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the view preferences.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"custom_view_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the `linear_custom_view` these preferences belong to. Changing it " +
					"replaces the resource — Linear's `viewPreferencesUpdate` only takes the preferences themselves.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"view_type": schema.StringAttribute{
				MarkdownDescription: "Which Linear view surface the preferences apply to, e.g. `customView`, " +
					"`board`, `project`, `cycle`. Changing it replaces the resource.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Scope of the preferences. Only `organization` — workspace-wide preferences — " +
					"is managed here; per-user preferences are personal state, not configuration.",
				Computed:      true,
				PlanModifiers: keepString(),
			},
			"preferences_json": schema.StringAttribute{
				MarkdownDescription: "Display preferences as a JSON object, e.g. " +
					"`jsonencode({ layout = \"board\", issueGrouping = \"assignee\" })`. Keys are Linear's own " +
					"`ViewPreferencesValues` field names. Compared semantically.",
				Required:      true,
				CustomType:    jsontypes.NormalizedType{},
				PlanModifiers: keepJSON(),
			},
		},
	}
}

func (r *viewPreferencesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := resourceClient(req, resp); ok {
		r.client = c
	}
}

func (r *viewPreferencesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan viewPreferencesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := map[string]any{
		"customViewId": plan.CustomViewID.ValueString(),
		"viewType":     plan.ViewType.ValueString(),
		"type":         "organization",
	}
	if err := putJSON(in, "preferences", plan.PreferencesJSON, false); err != nil {
		resp.Diagnostics.AddError("Invalid Linear view preferences", err.Error())
		return
	}

	doc := "mutation viewPreferencesCreate($input: ViewPreferencesCreateInput!) {\n" +
		"  viewPreferencesCreate(input: $input) {\n    viewPreferences { id type viewType }\n  }\n}"
	created, err := r.mutate(ctx, doc, map[string]any{"input": in}, "viewPreferencesCreate")
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Linear view preferences", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)
	plan.Type = types.StringValue(created.Type)
	plan.ViewType = types.StringValue(created.ViewType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *viewPreferencesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state viewPreferencesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only the keys under management are selected — see the note at the top.
	keys, err := managedPreferenceKeys(state.PreferencesJSON)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Linear view preferences", err.Error())
		return
	}

	selection := "id type viewType"
	if len(keys) > 0 {
		selection += " preferences { " + strings.Join(keys, " ") + " }"
	}
	doc := fmt.Sprintf("query customView($id: String!) {\n  customView(id: $id) {\n    organizationViewPreferences { %s }\n  }\n}", selection)

	var data map[string]json.RawMessage
	if err := r.client.Query(ctx, doc, map[string]any{"id": state.CustomViewID.ValueString()}, &data); err != nil {
		if client.NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Linear view preferences", err.Error())
		return
	}

	var view map[string]json.RawMessage
	if err := decodeField(data, "customView", &view); err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	var prefs struct {
		ID          string          `json:"id"`
		Type        string          `json:"type"`
		ViewType    string          `json:"viewType"`
		Preferences json.RawMessage `json:"preferences"`
	}
	// A view whose organization preferences were deleted returns null here, which
	// is this resource's not-found.
	if err := decodeField(view, "organizationViewPreferences", &prefs); err != nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(prefs.ID)
	state.Type = types.StringValue(prefs.Type)
	state.ViewType = types.StringValue(prefs.ViewType)
	if len(prefs.Preferences) > 0 {
		state.PreferencesJSON = jsonAttr(prefs.Preferences)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *viewPreferencesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan viewPreferencesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := map[string]any{}
	if err := putJSON(in, "preferences", plan.PreferencesJSON, false); err != nil {
		resp.Diagnostics.AddError("Invalid Linear view preferences", err.Error())
		return
	}

	doc := "mutation viewPreferencesUpdate($id: String!, $input: ViewPreferencesUpdateInput!) {\n" +
		"  viewPreferencesUpdate(id: $id, input: $input) {\n    viewPreferences { id type viewType }\n  }\n}"
	updated, err := r.mutate(ctx, doc, map[string]any{"id": plan.ID.ValueString(), "input": in}, "viewPreferencesUpdate")
	if err != nil {
		resp.Diagnostics.AddError("Unable to update Linear view preferences", err.Error())
		return
	}
	plan.Type = types.StringValue(updated.Type)
	plan.ViewType = types.StringValue(updated.ViewType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *viewPreferencesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state viewPreferencesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	doc := "mutation viewPreferencesDelete($id: String!) {\n  viewPreferencesDelete(id: $id) { success }\n}"
	if err := r.client.Mutate(ctx, doc, map[string]any{"id": state.ID.ValueString()}, nil); err != nil && !client.NotFound(err) {
		resp.Diagnostics.AddError("Unable to delete Linear view preferences", err.Error())
	}
}

// ImportState takes the custom view UUID, not the preferences UUID: the read
// path goes through the view, and the preferences id is only reachable from
// there. The preferences themselves come back empty on import, since there is no
// configuration yet to say which keys to select — the first plan fills them.
func (r *viewPreferencesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("custom_view_id"), req, resp)
}

type viewPreferencesPayload struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	ViewType string `json:"viewType"`
}

func (r *viewPreferencesResource) mutate(ctx context.Context, doc string, vars map[string]any, op string) (*viewPreferencesPayload, error) {
	var data map[string]json.RawMessage
	if err := r.client.Mutate(ctx, doc, vars, &data); err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if err := decodeField(data, op, &payload); err != nil {
		return nil, err
	}
	var out viewPreferencesPayload
	if err := decodeField(payload, "viewPreferences", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// managedPreferenceKeys returns the top-level keys of the preferences attribute,
// sorted so the generated query is stable. Keys are validated as GraphQL field
// names: they are interpolated into a query document, and a key carrying braces
// would let a configuration rewrite the query.
func managedPreferenceKeys(v jsontypes.Normalized) ([]string, error) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(v.ValueString()), &obj); err != nil {
		return nil, fmt.Errorf("parsing preferences_json: %w", err)
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		if !isGraphQLFieldName(k) {
			return nil, fmt.Errorf("preferences_json key %q is not a valid Linear preference name", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func isGraphQLFieldName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
