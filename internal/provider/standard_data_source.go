package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// Data sources differ from resources in almost everything except the plumbing:
// metadata, schema, configure. standardDataSource carries that plumbing so each
// data source file is its schema, its model and its read.

var (
	_ datasource.DataSource              = (*standardDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*standardDataSource)(nil)
)

type standardDataSource struct {
	client *client.Client

	// typeName is the data source type without the provider prefix, e.g. "team".
	typeName string
	// kind names the entity in diagnostics, e.g. "team".
	kind string
	// schema returns the data source schema.
	schema func() schema.Schema
	// newModel returns a zero model the config is read into and the state
	// written from.
	newModel func() any
	// read fills the model from the API.
	read func(ctx context.Context, c *client.Client, model any) error
}

func (d *standardDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.typeName
}

func (d *standardDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = d.schema()
}

func (d *standardDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := dataSourceClient(req, resp); ok {
		d.client = c
	}
}

func (d *standardDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	model := d.newModel()
	resp.Diagnostics.Append(req.Config.Get(ctx, model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.read(ctx, d.client, model); err != nil {
		resp.Diagnostics.AddError("Unable to read Linear "+d.kind, err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}
