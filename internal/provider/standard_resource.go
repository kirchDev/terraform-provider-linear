package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// Most Linear entities are CRUD-identical: create with an input object, read by
// id, update by id with a partial input, delete (or archive) by id, import by
// id. Writing that out per resource would be ~150 lines of the same code twenty
// times over, so standardResource implements it once and each resource file
// contributes only what actually differs — its schema, its state model, and how
// that model maps to and from the GraphQL shape.
//
// Resources that do not fit — the workspace_settings singleton, anything with a
// composite import id or a non-standard mutation — implement resource.Resource
// directly. Bending them into this type would cost more than it saves.

// crudModel is the state model of a standardResource.
type crudModel interface {
	// id returns the entity UUID currently in state.
	id() string
	// input builds the GraphQL create or update input from the model. forUpdate
	// selects the update input, where an attribute the user removed has to be
	// sent as an explicit null rather than omitted.
	input(ctx context.Context, forUpdate bool) map[string]any
	// decode fills the model from an entity as the API returned it.
	decode(ctx context.Context, raw json.RawMessage) error
}

// createThenUpdate is implemented by models of entities whose create input is
// narrower than their update input — Linear's TeamCreateInput, for instance,
// omits a dozen fields TeamUpdateInput accepts. Without the follow-up update
// those attributes would be silently dropped on the first apply and only take
// effect on the second.
type createThenUpdate interface {
	// needsUpdateAfterCreate reports whether the plan sets any attribute that
	// only exists on the update input.
	needsUpdateAfterCreate() bool
}

var (
	_ resource.Resource                = (*standardResource)(nil)
	_ resource.ResourceWithConfigure   = (*standardResource)(nil)
	_ resource.ResourceWithImportState = (*standardResource)(nil)
)

type standardResource struct {
	client *client.Client

	// entity is the GraphQL surface: name, selection set, delete verb.
	entity entity
	// typeName is the resource type without the provider prefix, e.g.
	// "workflow_state" for linear_workflow_state.
	typeName string
	// kind names the entity in diagnostics, e.g. "workflow state".
	kind string
	// schema returns the resource schema.
	schema func() schema.Schema
	// newModel returns a zero state model to decode plan and state into.
	newModel func() crudModel
	// deleteMsg overrides the delete diagnostic summary for entities Linear
	// archives rather than deletes.
	deleteMsg string
}

func (r *standardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.typeName
}

func (r *standardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = r.schema()
}

func (r *standardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := resourceClient(req, resp); ok {
		r.client = c
	}
}

func (r *standardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	plan := r.newModel()
	resp.Diagnostics.Append(req.Plan.Get(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var raw json.RawMessage
	if err := r.entity.create(ctx, r.client, plan.input(ctx, false), nil, &raw); err != nil {
		resp.Diagnostics.AddError("Unable to create Linear "+r.kind, err.Error())
		return
	}
	if err := plan.decode(ctx, raw); err != nil {
		resp.Diagnostics.AddError("Unable to read Linear "+r.kind+" after create", err.Error())
		return
	}

	// decode overwrote the plan with what the create returned, so the attributes
	// the create input could not carry have to be re-read from the plan before
	// the follow-up update sends them.
	if m, ok := plan.(createThenUpdate); ok && m.needsUpdateAfterCreate() {
		id := plan.id()
		resp.Diagnostics.Append(req.Plan.Get(ctx, plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.entity.update(ctx, r.client, id, plan.input(ctx, true), &raw); err != nil {
			resp.Diagnostics.AddError("Unable to apply Linear "+r.kind+" settings after create", err.Error())
			return
		}
		if err := plan.decode(ctx, raw); err != nil {
			resp.Diagnostics.AddError("Unable to read Linear "+r.kind+" after create", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *standardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	state := r.newModel()
	resp.Diagnostics.Append(req.State.Get(ctx, state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var raw json.RawMessage
	if err := r.entity.read(ctx, r.client, state.id(), &raw); err != nil {
		// Linear reports a missing entity as EntityNotFoundError inside an HTTP
		// 200. Dropping it from state here is what keeps the next plan from dying
		// at refresh.
		if client.NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Linear "+r.kind, err.Error())
		return
	}
	if err := state.decode(ctx, raw); err != nil {
		resp.Diagnostics.AddError("Unable to read Linear "+r.kind, err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *standardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	plan := r.newModel()
	resp.Diagnostics.Append(req.Plan.Get(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var raw json.RawMessage
	if err := r.entity.update(ctx, r.client, plan.id(), plan.input(ctx, true), &raw); err != nil {
		resp.Diagnostics.AddError("Unable to update Linear "+r.kind, err.Error())
		return
	}
	if err := plan.decode(ctx, raw); err != nil {
		resp.Diagnostics.AddError("Unable to read Linear "+r.kind+" after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *standardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	state := r.newModel()
	resp.Diagnostics.Append(req.State.Get(ctx, state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.entity.remove(ctx, r.client, state.id()); err != nil {
		msg := r.deleteMsg
		if msg == "" {
			msg = "Unable to delete Linear " + r.kind
		}
		resp.Diagnostics.AddError(msg, err.Error())
	}
}

// ImportState takes the entity UUID; every other attribute comes from the read
// the framework runs straight afterwards.
func (r *standardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
