package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// Optional + Computed is this provider's answer to "Linear defaults this one
// itself": the configuration may set it, and leaving it out has to mean keep
// what is live rather than null it. That is a statement about the *apply* — and
// without help the *plan* contradicts it.
//
// The framework marks a Computed attribute unknown from the **configuration**,
// not from the value the plan already carries: every Computed attribute whose
// configuration value is null becomes "(known after apply)" as soon as anything
// else on the resource changes at all (MarkComputedNilsAsUnknown, guarded by
// the plan differing from prior state). So renaming a team turned all thirty of
// its unset settings into "(known after apply)" — values that were known, sat
// in state, and were about to be left exactly as they were.
//
// The cost is not cosmetic. A plan is what a practitioner reads before an
// irreversible apply, and a resource that reports thirty changes when one thing
// changed trains the reader to skim precisely the output that must not be
// skimmed.
//
// UseStateForUnknown is the framework's answer, and it is a narrow one: it fires
// only where the plan value is unknown, the configuration value is not, and the
// prior state is non-null — so it never overwrites a configured value, and it
// never invents one for a resource being created. What it plans is the state as
// the *refresh* left it, so drift is still detected the only place it can be:
// in Read.
//
// keepBool and friends carry it, so an attribute opts in by naming one rather
// than by repeating the literal. Extra modifiers an attribute needs of its own
// — RequiresReplace, say — go through as arguments.

// keepBool returns the plan modifiers an Optional+Computed bool carries.
func keepBool(extra ...planmodifier.Bool) []planmodifier.Bool {
	return append([]planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}, extra...)
}

// keepString returns the plan modifiers an Optional+Computed string carries.
func keepString(extra ...planmodifier.String) []planmodifier.String {
	return append([]planmodifier.String{stringplanmodifier.UseStateForUnknown()}, extra...)
}

// keepFloat returns the plan modifiers an Optional+Computed float carries.
func keepFloat(extra ...planmodifier.Float64) []planmodifier.Float64 {
	return append([]planmodifier.Float64{float64planmodifier.UseStateForUnknown()}, extra...)
}

// keepInt returns the plan modifiers an Optional+Computed int carries.
func keepInt(extra ...planmodifier.Int64) []planmodifier.Int64 {
	return append([]planmodifier.Int64{int64planmodifier.UseStateForUnknown()}, extra...)
}

// keepList returns the plan modifiers an Optional+Computed list carries.
func keepList(extra ...planmodifier.List) []planmodifier.List {
	return append([]planmodifier.List{listplanmodifier.UseStateForUnknown()}, extra...)
}

// keepSet returns the plan modifiers an Optional+Computed set carries.
func keepSet(extra ...planmodifier.Set) []planmodifier.Set {
	return append([]planmodifier.Set{setplanmodifier.UseStateForUnknown()}, extra...)
}

// keepJSON returns the plan modifiers a jsontypes.Normalized attribute carries.
//
// The unknown half is keepString's, for the same reason. The second modifier
// answers a different half of the same promise: every one of these attributes is
// documented as "compared semantically", and on the apply it is — Linear
// normalises what it stores, so the value that comes back is the same document
// re-serialised, and both the update diff and the framework's own consistency
// check compare the parsed documents rather than the bytes.
//
// The plan is where that stops being true. terraform-plugin-framework applies a
// custom type's semantic equality in Create, Read and Update, and nowhere in
// PlanResourceChange — so the planned value is exactly the bytes the
// configuration wrote, and Terraform renders it against a differently-formatted
// state as `# whitespace changes`.
//
// Every path where the two spellings can diverge ends here: an import, where
// state holds Linear's serialisation and the configuration holds a
// practitioner's, and any configuration reformatted after the fact. Nothing
// converges it, either — Read keeps the prior value precisely because it is
// semantically equal, so the plan repeats unchanged forever.
func keepJSON(extra ...planmodifier.String) []planmodifier.String {
	return append([]planmodifier.String{
		stringplanmodifier.UseStateForUnknown(),
		semanticallyEqualJSON{},
	}, extra...)
}

// semanticallyEqualJSON plans the document already in state whenever the one the
// configuration asks for differs from it in formatting alone.
type semanticallyEqualJSON struct{}

func (semanticallyEqualJSON) Description(_ context.Context) string {
	return "Keeps the value in state when the configuration differs from it in JSON formatting alone."
}

func (m semanticallyEqualJSON) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (semanticallyEqualJSON) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Nothing to keep while the resource is being created, and nothing to plan
	// while it is being destroyed.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	// A null or unknown on either side is not a formatting difference. The
	// unknown case is keepString's modifier's, and it has already run.
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() ||
		req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	// The attribute's own comparison, rather than a second implementation of it:
	// this has to agree with what the framework does after the apply, or the
	// plan and the consistency check disagree about the same two documents.
	equal, diags := jsontypes.NewNormalizedValue(req.StateValue.ValueString()).
		StringSemanticEquals(ctx, jsontypes.NewNormalizedValue(req.PlanValue.ValueString()))
	if diags.HasError() || !equal {
		// A document that cannot be parsed is left alone rather than reported:
		// the custom type already validates the configuration's JSON with a
		// message of its own, and the plan is not the place to fail over
		// something the provider itself wrote to state.
		return
	}

	resp.PlanValue = req.StateValue
}
