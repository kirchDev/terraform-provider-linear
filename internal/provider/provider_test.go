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
