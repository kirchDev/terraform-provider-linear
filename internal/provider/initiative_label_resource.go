package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Initiative labels have no team scope — initiatives are workspace-level, so the
// label is too.
var initiativeLabelEntity = entity{
	name:   "initiativeLabel",
	fields: `id name color description isGroup parent { id }`,
}

// NewInitiativeLabelResource returns a new linear_initiative_label resource.
func NewInitiativeLabelResource() resource.Resource {
	return &standardResource{
		entity:   initiativeLabelEntity,
		typeName: "initiative_label",
		kind:     "initiative label",
		schema:   func() schema.Schema { return groupableLabelSchema("initiative", false) },
		newModel: func() crudModel { return &initiativeLabelModel{} },
	}
}

type initiativeLabelModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Color       types.String `tfsdk:"color"`
	Description types.String `tfsdk:"description"`
	IsGroup     types.Bool   `tfsdk:"is_group"`
	ParentID    types.String `tfsdk:"parent_id"`
}

func (m *initiativeLabelModel) id() string { return m.ID.ValueString() }

func (m *initiativeLabelModel) decode(_ context.Context, raw json.RawMessage) error {
	var a issueLabelAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Color = stringOrNull(a.Color)
	m.Description = types.StringPointerValue(a.Description)
	m.IsGroup = types.BoolValue(a.IsGroup)
	m.ParentID = refID(a.Parent)
	return nil
}

func (m *initiativeLabelModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}
	putString(in, "color", m.Color, false)
	putString(in, "description", m.Description, forUpdate)
	putString(in, "parentId", m.ParentID, forUpdate)
	if !forUpdate {
		putBool(in, "isGroup", m.IsGroup, false)
	}
	return in
}
