package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// resourceClient unwraps the *client.Client the provider handed down. A nil
// ProviderData is normal — the framework calls Configure before the provider is
// configured during validation.
func resourceClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) (*client.Client, bool) {
	if req.ProviderData == nil {
		return nil, false
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return nil, false
	}
	return c, true
}

// dataSourceClient is resourceClient for data sources.
func dataSourceClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) (*client.Client, bool) {
	if req.ProviderData == nil {
		return nil, false
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return nil, false
	}
	return c, true
}

// --- input builders ---
//
// A GraphQL input object only carries the fields it lists, so an omitted field
// keeps its server-side value. That is what Create wants, but on Update it turns
// "the user removed the attribute from the config" into a silent no-op. The
// `clear` flag therefore sends an explicit null on Update so Linear resets the
// field. Optional+Computed attributes pass clear=false: for those, an absent
// value legitimately means "keep whatever is live".

func putString(in map[string]any, key string, v types.String, clear bool) {
	switch {
	case v.IsUnknown():
	case !v.IsNull():
		in[key] = v.ValueString()
	case clear:
		in[key] = nil
	}
}

// putRef sets key from a UUID-typed reference attribute, reading `""` as an
// explicit clear and sending it to Linear as a null.
//
// It is the second trigger for the null `clear` above sends, and it exists
// because Optional + Computed took the first one away. "An attribute the
// configuration does not mention keeps its live value" is the right default for
// a workspace a human also edits in the UI, but it leaves no way to say *unset
// this*: HCL cannot tell `x = null` from omitting `x`, so both now mean keep.
// Free text still has an escape — `description = ""` is an empty description —
// and a reference had none at all, which made a nested team impossible to
// un-nest through the provider.
//
// `""` is not a UUID Linear would ever store, so on a reference it can only mean
// one thing, and that is what it is read as. Only references get this: on free
// text `""` is a value, and `description = ""` has to keep meaning an empty
// description rather than "restore whatever is live".
//
// Attributes written through here state the mechanism in their own description
// via clearWithEmptyString, and keepCleared is the other half — the round trip
// back into state.
func putRef(in map[string]any, key string, v types.String) {
	switch {
	case v.IsUnknown(), v.IsNull():
	case v.ValueString() == "":
		in[key] = nil
	default:
		in[key] = v.ValueString()
	}
}

// clearWithEmptyString is the sentence every attribute putRef writes carries, so
// the mechanism reaches the practitioner reading the registry docs and not only
// the next person reading helpers.go. Shared rather than retyped: eleven
// hand-written variants would drift, and a reference added later should inherit
// the wording by construction.
const clearWithEmptyString = " Set it to `\"\"` to unset the reference — leaving the attribute out of the " +
	"configuration keeps whatever is live, so `\"\"` is how a configuration asks for no reference at all."

func putBool(in map[string]any, key string, v types.Bool, clear bool) {
	switch {
	case v.IsUnknown():
	case !v.IsNull():
		in[key] = v.ValueBool()
	case clear:
		in[key] = nil
	}
}

func putInt(in map[string]any, key string, v types.Int64, clear bool) {
	switch {
	case v.IsUnknown():
	case !v.IsNull():
		in[key] = v.ValueInt64()
	case clear:
		in[key] = nil
	}
}

func putFloat(in map[string]any, key string, v types.Float64, clear bool) {
	switch {
	case v.IsUnknown():
	case !v.IsNull():
		in[key] = v.ValueFloat64()
	case clear:
		in[key] = nil
	}
}

// putStringList sets key from a tfsdk list of strings. An empty (but non-null)
// list is sent as an empty array — clearing a collection is a real intent and
// differs from omitting it.
func putStringList(ctx context.Context, in map[string]any, key string, v types.List, clear bool) error {
	if v.IsUnknown() {
		return nil
	}
	if v.IsNull() {
		if clear {
			in[key] = nil
		}
		return nil
	}
	var out []string
	if d := v.ElementsAs(ctx, &out, false); d.HasError() {
		return fmt.Errorf("reading %s", key)
	}
	in[key] = orEmpty(out)
	return nil
}

// putStringSet is putStringList for an unordered collection.
func putStringSet(ctx context.Context, in map[string]any, key string, v types.Set, clear bool) error {
	if v.IsUnknown() {
		return nil
	}
	if v.IsNull() {
		if clear {
			in[key] = nil
		}
		return nil
	}
	var out []string
	if d := v.ElementsAs(ctx, &out, false); d.HasError() {
		return fmt.Errorf("reading %s", key)
	}
	in[key] = orEmpty(out)
	return nil
}

// putFloatList sets key from a tfsdk list of numbers, e.g. the workspace's
// working days. Never cleared — the workspace singleton keeps live values for
// anything the config omits.
func putFloatList(ctx context.Context, in map[string]any, key string, v types.List) error {
	if v.IsUnknown() || v.IsNull() {
		return nil
	}
	var out []float64
	if d := v.ElementsAs(ctx, &out, false); d.HasError() {
		return fmt.Errorf("reading %s", key)
	}
	in[key] = orEmptyFloats(out)
	return nil
}

func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func orEmptyFloats(in []float64) []float64 {
	if in == nil {
		return []float64{}
	}
	return in
}

// errBuildingList is the failure of turning an API slice into a tfsdk
// collection — only reachable on a type mismatch, so the message stays short.
func errBuildingList(attr string) error {
	return fmt.Errorf("building %s", attr)
}

// putJSON sets key from a `*_json` attribute. The GraphQL field is a JSON scalar
// and expects the decoded value, not the string holding it, so the attribute is
// unmarshalled on the way out.
func putJSON(in map[string]any, key string, v jsontypes.Normalized, clear bool) error {
	if v.IsUnknown() {
		return nil
	}
	if v.IsNull() {
		if clear {
			in[key] = nil
		}
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(v.ValueString()), &decoded); err != nil {
		return fmt.Errorf("parsing %s: %w", key, err)
	}
	in[key] = decoded
	return nil
}

// jsonAttr maps a JSON scalar the API returned back into a `*_json` attribute.
//
// The value is stored exactly as Linear serialised it. That would drift against
// the user's own formatting on every plan if the attribute were a plain string —
// which is why these attributes are jsontypes.Normalized, comparing semantically
// rather than byte for byte. Linear also normalises some of these objects
// server-side (filterData notably), so even a byte-identical round-trip is not
// something the provider can rely on.
func jsonAttr(raw json.RawMessage) jsontypes.Normalized {
	if len(raw) == 0 || string(raw) == "null" {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(string(raw))
}

// --- read helpers ---

// stringOrNull maps an API string to state, turning "" into null so an optional
// field Linear returns as an empty string does not read as drift.
func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// listOfStrings builds a tfsdk list from a slice; a nil slice becomes an empty
// list, not null, so a collection Linear reports as absent still round-trips.
func listOfStrings(ctx context.Context, in []string) (types.List, error) {
	if in == nil {
		in = []string{}
	}
	l, d := types.ListValueFrom(ctx, types.StringType, in)
	if d.HasError() {
		return types.ListNull(types.StringType), fmt.Errorf("building string list")
	}
	return l, nil
}

// setOfStrings is listOfStrings for an unordered collection.
func setOfStrings(ctx context.Context, in []string) (types.Set, error) {
	if in == nil {
		in = []string{}
	}
	s, d := types.SetValueFrom(ctx, types.StringType, in)
	if d.HasError() {
		return types.SetNull(types.StringType), fmt.Errorf("building string set")
	}
	return s, nil
}

// ref is the shape Linear returns for a nested relation this provider only
// stores the id of.
type ref struct {
	ID string `json:"id"`
}

// refID maps an optional relation to its id, or null when the relation is unset.
func refID(r *ref) types.String {
	if r == nil {
		return types.StringNull()
	}
	return types.StringValue(r.ID)
}

// keepCleared maps a reference Linear answered with back into state, preserving
// the `""` putRef sent as an explicit null.
//
// Linear answers a cleared reference with no reference at all, and mapping that
// straight to null would contradict the `""` Terraform planned — the framework
// fails an apply whose result differs from a value it planned as known, so the
// clear would end in "provider produced inconsistent result after apply". The
// refresh needs it just as much: rewriting `""` to null there puts state and
// configuration back out of step and reopens the same diff on every plan.
//
// current is whatever the model already holds — the planned value during an
// apply, the prior state during a read. A reference Linear does report always
// wins, so a parent set in the UI behind Terraform's back is still drift.
func keepCleared(current, fromAPI types.String) types.String {
	if fromAPI.IsNull() && !current.IsNull() && !current.IsUnknown() && current.ValueString() == "" {
		return current
	}
	return fromAPI
}
