package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// tfplugindocs renders docs/ from the schema plus whatever sits under
// examples/. A resource with no example silently gets a page without one, which
// nobody notices until the registry shows it. These tests make that a build
// failure instead.

func examplesDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("resolving examples dir: %v", err)
	}
	return dir
}

func TestEveryResourceHasAnExample(t *testing.T) {
	ctx := context.Background()
	dir := examplesDir(t)

	for _, newResource := range New("test")().Resources(ctx) {
		var meta resource.MetadataResponse
		newResource().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "linear"}, &meta)

		for _, file := range []string{"resource.tf", "import.sh"} {
			path := filepath.Join(dir, "resources", meta.TypeName, file)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s: missing examples/resources/%s/%s", meta.TypeName, meta.TypeName, file)
			}
		}
	}
}

func TestEveryDataSourceHasAnExample(t *testing.T) {
	ctx := context.Background()
	dir := examplesDir(t)

	for _, newDataSource := range New("test")().DataSources(ctx) {
		var meta datasource.MetadataResponse
		newDataSource().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "linear"}, &meta)

		path := filepath.Join(dir, "data-sources", meta.TypeName, "data-source.tf")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: missing examples/data-sources/%s/data-source.tf", meta.TypeName, meta.TypeName)
		}
	}
}

// The reverse direction: an example whose resource was renamed or dropped would
// otherwise sit there forever, documenting something that no longer exists.
func TestEveryExampleHasAResource(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	registered := map[string]bool{}
	for _, newResource := range p.Resources(ctx) {
		var meta resource.MetadataResponse
		newResource().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "linear"}, &meta)
		registered[meta.TypeName] = true
	}

	entries, err := os.ReadDir(filepath.Join(examplesDir(t), "resources"))
	if err != nil {
		t.Fatalf("reading examples/resources: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() && !registered[e.Name()] {
			t.Errorf("examples/resources/%s has no registered resource", e.Name())
		}
	}
}

func TestEveryDataSourceExampleHasADataSource(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	registered := map[string]bool{}
	for _, newDataSource := range p.DataSources(ctx) {
		var meta datasource.MetadataResponse
		newDataSource().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "linear"}, &meta)
		registered[meta.TypeName] = true
	}

	entries, err := os.ReadDir(filepath.Join(examplesDir(t), "data-sources"))
	if err != nil {
		t.Fatalf("reading examples/data-sources: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() && !registered[e.Name()] {
			t.Errorf("examples/data-sources/%s has no registered data source", e.Name())
		}
	}
}
