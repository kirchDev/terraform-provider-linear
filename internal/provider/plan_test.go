package provider

import (
	"context"
	"reflect"
	"sort"
	"testing"

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

// derivedFromAnotherAttribute names the Optional+Computed attributes Linear
// computes from a *second attribute of the same resource*, rather than
// defaulting on its own. For those, and only those, "(known after apply)" is
// the honest plan: change the attribute they are derived from and the value
// really is not knowable until Linear has answered.
//
// Everything else Linear defaults independently and then leaves alone, so its
// prior value is its planned value — see plan.go.
var derivedFromAnotherAttribute = map[string]string{
	"linear_team.key":                     "Linear derives the key from the name when unset",
	"linear_release_pipeline.slug_id":     "Linear derives the slug from the name when unset",
	"linear_customer_status.name":         "name and display_name fill in for each other",
	"linear_customer_status.display_name": "name and display_name fill in for each other",
	"linear_customer_tier.name":           "name and display_name fill in for each other",
	"linear_customer_tier.display_name":   "name and display_name fill in for each other",
}

// An Optional+Computed attribute with no configuration value plans as "(known
// after apply)" unless the provider says otherwise, and it does so on every
// attribute at once the moment anything on the resource changes. That is a
// property of the framework rather than of any one resource, so the guard is
// over the whole provider: a resource added later inherits the same trap, and
// the schema is where it is either avoided or not.
//
// Two kinds of attribute are exempt. One with a Default is never marked unknown
// in the first place — the framework skips it — so a plan modifier would be
// dead weight. One Linear derives from another attribute is genuinely unknown
// until the apply, and is listed above with the reason.
func TestOptionalComputedAttributesKeepTheirPriorValue(t *testing.T) {
	t.Parallel()

	for typeName, s := range resourceSchemas(t) {
		names := make([]string, 0, len(s.Attributes))
		for name := range s.Attributes {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			attribute := s.Attributes[name]
			if !attribute.IsOptional() || !attribute.IsComputed() {
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
				t.Errorf("%s is Optional+Computed without UseStateForUnknown: leaving it out of the "+
					"configuration makes it plan as (known after apply) as soon as anything else on the "+
					"resource changes. Give it keepX() from plan.go, or list it in "+
					"derivedFromAnotherAttribute with the reason.", address)
			}
		}
	}
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
