package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Linear has no emojiUpdate — only create and delete. Every settable attribute
// is therefore RequiresReplace, which also means Update is never reached.
var emojiEntity = entity{
	name:   "emoji",
	fields: `id name url source`,
}

// NewEmojiResource returns a new linear_emoji resource.
func NewEmojiResource() resource.Resource {
	return &standardResource{
		entity:   emojiEntity,
		typeName: "emoji",
		kind:     "emoji",
		schema:   emojiSchema,
		newModel: func() crudModel { return &emojiModel{} },
	}
}

type emojiAttributes struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Source string `json:"source"`
}

type emojiModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	// URL is what the configuration asked Linear to fetch; HostedURL is where
	// Linear re-hosted it. Writing the hosted URL back into `url` would make the
	// applied state differ from the plan, which Terraform rejects outright.
	URL       types.String `tfsdk:"url"`
	HostedURL types.String `tfsdk:"hosted_url"`
	Source    types.String `tfsdk:"source"`
}

func (m *emojiModel) id() string { return m.ID.ValueString() }

func (m *emojiModel) decode(_ context.Context, raw json.RawMessage) error {
	var a emojiAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.HostedURL = types.StringValue(a.URL)
	m.Source = stringOrNull(a.Source)
	return nil
}

func (m *emojiModel) input(_ context.Context, _ bool) map[string]any {
	return map[string]any{
		"name": m.Name.ValueString(),
		"url":  m.URL.ValueString(),
	}
}

func emojiSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a custom emoji in the Linear workspace.\n\n" +
			"Linear has no update mutation for emoji, so changing either attribute replaces the resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the emoji.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name the emoji is typed as, without colons, e.g. `shipit`. Changing it " +
					"replaces the emoji.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "URL of the image Linear fetches. Changing it replaces the emoji.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"hosted_url": schema.StringAttribute{
				MarkdownDescription: "URL of Linear's own copy of the image, which is what the workspace serves.",
				Computed:            true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "Where the emoji came from, as Linear records it.",
				Computed:            true,
			},
		},
	}
}
