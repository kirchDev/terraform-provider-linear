package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// Schema validation is cheap and catches the mistakes that are otherwise only
// visible at `tofu plan` against a real workspace: an attribute that is neither
// optional, required nor computed, a name that is not snake_case, a description
// that never got written.

func TestResourceSchemasAreValid(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	seen := map[string]bool{}
	for _, newResource := range p.Resources(ctx) {
		r := newResource()

		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "linear"}, &meta)
		if !strings.HasPrefix(meta.TypeName, "linear_") {
			t.Errorf("resource type %q does not start with linear_", meta.TypeName)
		}
		if seen[meta.TypeName] {
			t.Errorf("resource type %q is registered twice", meta.TypeName)
		}
		seen[meta.TypeName] = true

		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
		if schemaResp.Diagnostics.HasError() {
			t.Errorf("%s: schema diagnostics: %v", meta.TypeName, schemaResp.Diagnostics)
			continue
		}
		if d := schemaResp.Schema.ValidateImplementation(ctx); d.HasError() {
			t.Errorf("%s: invalid schema: %v", meta.TypeName, d)
		}

		if _, ok := r.(resource.ResourceWithConfigure); !ok {
			t.Errorf("%s does not implement ResourceWithConfigure, so it never gets a client", meta.TypeName)
		}
		if _, ok := r.(resource.ResourceWithImportState); !ok {
			t.Errorf("%s does not implement ImportState", meta.TypeName)
		}

		checkResourceAttributes(t, meta.TypeName, schemaResp.Schema)
	}

	if len(seen) == 0 {
		t.Fatal("the provider registers no resources")
	}
}

func checkResourceAttributes(t *testing.T, typeName string, s fwresource.Schema) {
	t.Helper()

	if s.MarkdownDescription == "" {
		t.Errorf("%s: schema has no MarkdownDescription", typeName)
	}
	if _, ok := s.Attributes["id"]; !ok {
		t.Errorf("%s: no id attribute, so ImportState has nothing to write to", typeName)
	}

	for name, attr := range s.Attributes {
		if !isSnakeCase(name) {
			t.Errorf("%s: attribute %q is not snake_case", typeName, name)
		}
		if attr.GetMarkdownDescription() == "" && attr.GetDescription() == "" {
			t.Errorf("%s: attribute %q has no description", typeName, name)
		}
		if !attr.IsRequired() && !attr.IsOptional() && !attr.IsComputed() {
			t.Errorf("%s: attribute %q is neither required, optional nor computed", typeName, name)
		}
	}
}

// An attribute Linear reports back must be Optional + Computed, never plain
// Optional. Terraform reads a null configuration value for a non-computed
// attribute as "make it null": the refresh reads the live value into state, the
// plan proposes state → null, and the apply sends an explicit null that erases
// it. That erases settings a human set in the Linear UI on the strength of a
// configuration that never mentioned them, which is data loss rather than drift.
//
// The one attribute that is plain Optional on purpose is the write-only kind —
// Linear accepts it on the input and never returns it. There is nothing to
// refresh it against, and Computed would end every apply in "provider produced
// inconsistent result after apply" because the planned value is not one Linear
// sends back. Those state **write-only** in their description, which is this
// repo's own convention and what this test reads them by.
//
// The second exemption is nullIsASettingNotAnAbsence below, where the update
// input sends the null on purpose.
//
// The rule is checked here, across every resource, rather than resource by
// resource: it is a property of the provider, and one file getting it wrong is
// exactly as expensive as all of them.
//
// Nested attributes are deliberately not walked. Every nested object in this
// provider hangs off a collection Linear replaces wholesale, so its elements are
// never the "attribute the configuration did not mention" this rule is about —
// the collection is either in the configuration or it is not.
func TestResourceOptionalAttributesAreComputedUnlessWriteOnly(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	for _, newResource := range p.Resources(ctx) {
		r := newResource()

		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "linear"}, &meta)

		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
		if schemaResp.Diagnostics.HasError() {
			// TestResourceSchemasAreValid is what reports a broken schema.
			continue
		}

		for name, attr := range schemaResp.Schema.Attributes {
			if !attr.IsOptional() || attr.IsComputed() || describesWriteOnly(attr) {
				continue
			}
			if nullIsASettingNotAnAbsence[meta.TypeName+"."+name] != "" {
				continue
			}
			t.Errorf("%s: attribute %q is Optional without Computed and is not documented write-only, "+
				"so an apply clears its live value whenever the configuration omits it",
				meta.TypeName, name)
		}
	}
}

// nullIsASettingNotAnAbsence lists the attributes whose *unset* state is a
// choice the resource sends to Linear as an explicit null, rather than an
// attribute the configuration merely did not mention. Computed would make that
// null unreachable — the plan would keep the live value forever and the mode
// could never be switched back — so plain Optional is correct for exactly these,
// and the reason is recorded here rather than left to be re-derived.
//
// The line is drawn by what the update input does, which is checkable: if
// input() sends nil for the attribute on purpose, it belongs here; if the null
// only means "not mentioned", the attribute is Optional + Computed. Nothing is
// added to this map because an apply was inconvenient — the settings the issue
// was reported against (a team's icon, its default issue state) are absences,
// not choices, and they are exactly what must not be listed.
var nullIsASettingNotAnAbsence = map[string]string{
	"linear_agent_skill.team_id":                   "a null teamId shares the skill workspace-wide",
	"linear_custom_view.team_id":                   "a null teamId widens a team view to the workspace",
	"linear_template.team_id":                      "a null teamId shares a team template across the workspace",
	"linear_git_automation_state.state_id":         "a null stateId is how a rule overrides a default with no action",
	"linear_git_automation_state.target_branch_id": "a null targetBranchId applies the rule to every branch",
	"linear_triage_responsibility.time_schedule_id": "a null timeScheduleId switches a rota back to a manual " +
		"selection",
	"linear_triage_responsibility.manual_user_ids": "a null manualSelection hands triage back to a rota",
}

// describesWriteOnly reports whether an attribute's own description says Linear
// never returns it. Being unreadable is the only thing that justifies a plain
// Optional attribute, and the description is where this provider records it — so
// the exemption is spelled out for the practitioner rather than kept in a list
// only the test can see.
func describesWriteOnly(attr fwresource.Attribute) bool {
	desc := attr.GetMarkdownDescription() + " " + attr.GetDescription()
	return strings.Contains(strings.ToLower(desc), "write-only")
}

func TestDataSourceSchemasAreValid(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	seen := map[string]bool{}
	for _, newDataSource := range p.DataSources(ctx) {
		d := newDataSource()

		var meta datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "linear"}, &meta)
		if !strings.HasPrefix(meta.TypeName, "linear_") {
			t.Errorf("data source type %q does not start with linear_", meta.TypeName)
		}
		if seen[meta.TypeName] {
			t.Errorf("data source type %q is registered twice", meta.TypeName)
		}
		seen[meta.TypeName] = true

		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		if schemaResp.Diagnostics.HasError() {
			t.Errorf("%s: schema diagnostics: %v", meta.TypeName, schemaResp.Diagnostics)
			continue
		}
		if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Errorf("%s: invalid schema: %v", meta.TypeName, diags)
		}
		if _, ok := d.(datasource.DataSourceWithConfigure); !ok {
			t.Errorf("%s does not implement DataSourceWithConfigure", meta.TypeName)
		}
		checkDataSourceAttributes(t, meta.TypeName, schemaResp.Schema)
	}
}

func checkDataSourceAttributes(t *testing.T, typeName string, s fwdatasource.Schema) {
	t.Helper()

	if s.MarkdownDescription == "" {
		t.Errorf("%s: schema has no MarkdownDescription", typeName)
	}
	for name, attr := range s.Attributes {
		if !isSnakeCase(name) {
			t.Errorf("%s: attribute %q is not snake_case", typeName, name)
		}
		if attr.GetMarkdownDescription() == "" && attr.GetDescription() == "" {
			t.Errorf("%s: attribute %q has no description", typeName, name)
		}
	}
}

func isSnakeCase(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return s[0] != '_' && s[len(s)-1] != '_'
}

func TestProviderSchemaIsValid(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	var resp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("provider schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["token"]; !ok {
		t.Error("provider schema has no token attribute")
	}
	if !resp.Schema.Attributes["token"].IsSensitive() {
		t.Error("the token attribute is not marked sensitive")
	}
}
