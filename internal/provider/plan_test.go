package provider

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// derivedFromAnotherAttribute names the Computed attributes Linear computes from
// a *second attribute of the same resource*, rather than defaulting on its own.
// For those, and only those, "(known after apply)" is the honest plan: change
// the attribute they are derived from and the value really is not knowable until
// Linear has answered.
//
// Everything else Linear defaults independently and then leaves alone, so its
// prior value is its planned value — see plan.go.
//
// Each entry says which attribute the value is derived from, because that is the
// claim being made and naming it is what makes it reviewable. Most are checkable
// against the attribute's own description; the three on linear_agent_skill are
// not — "URL slug of the skill" and "Whether the skill is shared beyond its
// owner" say nothing about a title or a team_id, so those entries read the
// resource's shape rather than a documented promise. Nothing here has been
// observed against a real workspace either. A wrong entry costs plan noise on
// that one resource — the attribute reads "(known after apply)" where it could
// have read its prior value — not a wrong value, so an inference is allowed to
// stand here as long as it is visible as one.
//
// A read-only attribute that is *not* derived from anything belongs in neither
// camp and simply carries keepX(): it cannot change, so its prior value is the
// only honest plan.
var derivedFromAnotherAttribute = map[string]string{
	"linear_team.key":                     "Linear derives the key from the name when unset",
	"linear_release_pipeline.slug_id":     "Linear derives the slug from the name when unset",
	"linear_customer_status.name":         "name and display_name fill in for each other",
	"linear_customer_status.display_name": "name and display_name fill in for each other",
	"linear_customer_tier.name":           "name and display_name fill in for each other",
	"linear_customer_tier.display_name":   "name and display_name fill in for each other",
	"linear_custom_view.model_name":       "Linear derives what the view lists from whichever filter is set",
	"linear_agent_skill.description":      "Linear derives the description from the body",
	"linear_agent_skill.slug_id":          "Linear derives the slug from the title",
	"linear_agent_skill.shared":           "follows team_id — a skill with no team is shared workspace-wide",
	"linear_emoji.hosted_url":             "Linear hosts its own copy of the image url points at",
	"linear_emoji.source":                 "Linear records where it fetched url from",
}

// A Computed attribute with no configuration value plans as "(known after
// apply)" unless the provider says otherwise, and it does so on every attribute
// at once the moment anything on the resource changes. That is a property of the
// framework rather than of any one resource, so the guard is over the whole
// provider: a resource added later inherits the same trap, and the schema is
// where it is either avoided or not.
//
// It reaches every Computed attribute, not only the Optional+Computed ones.
// Optional+Computed is how this provider says "Linear defaults this itself", and
// that is nearly all of them — but the framework marks a value unknown because
// the *configuration* holds none, and a read-only attribute holds none by
// construction. linear_workspace_settings.releases_enabled is the one that sat
// in that gap: Linear reports it and organizationUpdate cannot set it, so it is
// Computed alone, and it went unknown on every plan that changed anything at all
// while the narrower guard passed.
//
// Two kinds of attribute are exempt. One with a Default is never marked unknown
// in the first place — the framework skips it — so a plan modifier would be
// dead weight. One Linear derives from another attribute is genuinely unknown
// until the apply, and is listed above with the reason.
func TestComputedAttributesKeepTheirPriorValue(t *testing.T) {
	t.Parallel()

	for typeName, s := range resourceSchemas(t) {
		names := make([]string, 0, len(s.Attributes))
		for name := range s.Attributes {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			attribute := s.Attributes[name]
			if !attribute.IsComputed() {
				continue
			}

			address := typeName + "." + name
			modifiers, hasDefault := planModifiers(t, address, attribute)
			if hasDefault {
				continue
			}

			why, derived := derivedFromAnotherAttribute[address]
			switch {
			case derived && keepsState(modifiers):
				t.Errorf("%s carries UseStateForUnknown but is listed as derived (%s) — "+
					"drop it from derivedFromAnotherAttribute or from the schema", address, why)
			case !derived && !keepsState(modifiers):
				t.Errorf("%s is Computed without UseStateForUnknown: with no value in the "+
					"configuration it plans as (known after apply) as soon as anything else on the "+
					"resource changes. Give it keepX() from plan.go, or list it in "+
					"derivedFromAnotherAttribute with the reason.", address)
			}
		}
	}
}

// Every JSON attribute is documented as compared semantically, and until the
// plan says so too that claim holds only after the apply: the framework runs a
// custom type's semantic equality in Create, Read and Update, and nowhere in
// PlanResourceChange. So a document that differs from state in formatting alone
// plans as a change, forever — see keepJSON in plan.go.
//
// The guard is over the whole provider for the same reason as the one above:
// this is a property of jsontypes.Normalized rather than of any one attribute,
// so every resource that reaches for the type inherits it. Data sources are out
// of scope — they have no plan to modify.
func TestJSONAttributesAreComparedSemanticallyInThePlan(t *testing.T) {
	t.Parallel()

	found := 0
	for typeName, s := range resourceSchemas(t) {
		names := make([]string, 0, len(s.Attributes))
		for name := range s.Attributes {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			attribute, ok := s.Attributes[name].(schema.StringAttribute)
			if !ok || attribute.CustomType == nil {
				continue
			}
			if _, isJSON := attribute.CustomType.(jsontypes.NormalizedType); !isJSON {
				continue
			}

			found++
			if !comparesJSONSemantically(anySlice(attribute.PlanModifiers)) {
				t.Errorf("%s.%s is a jsontypes.Normalized attribute without the semantic-equality plan "+
					"modifier: a value differing from state in whitespace or key order alone plans as a "+
					"change on every plan, which the attribute's own description says it does not. "+
					"Give it keepJSON() from plan.go.", typeName, name)
			}
		}
	}

	if found == 0 {
		t.Fatal("no jsontypes.Normalized attributes found — the guard would pass vacuously")
	}
}

func comparesJSONSemantically(modifiers []any) bool {
	for _, m := range modifiers {
		if _, ok := m.(semanticallyEqualJSON); ok {
			return true
		}
	}
	return false
}

// resourceSchemas returns every registered resource's schema, keyed by type
// name. The provider is the list of what ships, so nothing new can be added
// without this guard seeing it.
func resourceSchemas(t *testing.T) map[string]schema.Schema {
	t.Helper()
	ctx := context.Background()

	schemas := map[string]schema.Schema{}
	for _, newResource := range New("test")().Resources(ctx) {
		r := newResource()

		var metadata resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "linear"}, &metadata)

		var response resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("schema of %s: %v", metadata.TypeName, response.Diagnostics)
		}
		schemas[metadata.TypeName] = response.Schema
	}
	if len(schemas) == 0 {
		t.Fatal("no resources registered — the guard would pass vacuously")
	}
	return schemas
}

// planModifiers returns an attribute's plan modifiers and whether it carries a
// default. Both live on the concrete attribute types rather than on the
// interface, so reading them is a type switch.
func planModifiers(t *testing.T, address string, attribute schema.Attribute) (modifiers []any, hasDefault bool) {
	t.Helper()

	switch a := attribute.(type) {
	case schema.BoolAttribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	case schema.StringAttribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	case schema.Int64Attribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	case schema.Float64Attribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	case schema.ListAttribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	case schema.SetAttribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	case schema.MapAttribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	case schema.ObjectAttribute:
		return anySlice(a.PlanModifiers), a.Default != nil
	}

	// A type this switch does not know is not a pass: it is an attribute the
	// guard cannot see, which is the one outcome worse than a failing guard.
	t.Fatalf("%s: %T is a type this guard cannot read plan modifiers off — teach it the type "+
		"rather than letting the attribute through unchecked", address, attribute)
	return nil, false
}

func anySlice[T any](in []T) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}

// useStateForUnknown is one instance of each type's modifier, matched by type
// rather than by its description string: the description is prose the framework
// is free to reword, the type is the thing that does the work.
var useStateForUnknown = func() map[reflect.Type]bool {
	types := map[reflect.Type]bool{}
	for _, m := range []any{
		boolplanmodifier.UseStateForUnknown(),
		stringplanmodifier.UseStateForUnknown(),
		int64planmodifier.UseStateForUnknown(),
		float64planmodifier.UseStateForUnknown(),
		listplanmodifier.UseStateForUnknown(),
		setplanmodifier.UseStateForUnknown(),
		mapplanmodifier.UseStateForUnknown(),
		objectplanmodifier.UseStateForUnknown(),
	} {
		types[reflect.TypeOf(m)] = true
	}
	return types
}()

func keepsState(modifiers []any) bool {
	for _, m := range modifiers {
		if useStateForUnknown[reflect.TypeOf(m)] {
			return true
		}
	}
	return false
}
