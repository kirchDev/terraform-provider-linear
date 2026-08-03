package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The secret is deliberately not part of the read selection. Linear will return
// it, but a value that only ever travels outbound cannot leak through a refresh,
// a debug log or a `tofu show`. State keeps whatever the configuration set,
// which is enough to detect a change in the configuration itself.
var webhookEntity = entity{
	name:   "webhook",
	fields: `id url label enabled resourceTypes allPublicTeams team { id }`,
}

// NewWebhookResource returns a new linear_webhook resource.
func NewWebhookResource() resource.Resource {
	return &standardResource{
		entity:   webhookEntity,
		typeName: "webhook",
		kind:     "webhook",
		schema:   webhookSchema,
		newModel: func() crudModel { return &webhookModel{} },
	}
}

type webhookAttributes struct {
	ID             string   `json:"id"`
	URL            *string  `json:"url"`
	Label          *string  `json:"label"`
	Enabled        bool     `json:"enabled"`
	ResourceTypes  []string `json:"resourceTypes"`
	AllPublicTeams bool     `json:"allPublicTeams"`
	Team           *ref     `json:"team"`
}

type webhookModel struct {
	ID             types.String `tfsdk:"id"`
	TeamID         types.String `tfsdk:"team_id"`
	URL            types.String `tfsdk:"url"`
	Label          types.String `tfsdk:"label"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	ResourceTypes  types.Set    `tfsdk:"resource_types"`
	AllPublicTeams types.Bool   `tfsdk:"all_public_teams"`
	Secret         types.String `tfsdk:"secret"`
}

func (m *webhookModel) id() string { return m.ID.ValueString() }

func (m *webhookModel) decode(ctx context.Context, raw json.RawMessage) error {
	var a webhookAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.URL = types.StringPointerValue(a.URL)
	m.Label = types.StringPointerValue(a.Label)
	m.Enabled = types.BoolValue(a.Enabled)
	m.AllPublicTeams = types.BoolValue(a.AllPublicTeams)
	m.TeamID = refID(a.Team)

	set, err := setOfStrings(ctx, a.ResourceTypes)
	if err != nil {
		return err
	}
	m.ResourceTypes = set
	// Secret is left alone — see the note on webhookEntity.
	return nil
}

func (m *webhookModel) input(ctx context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"url": m.URL.ValueString()}
	putString(in, "label", m.Label, forUpdate)
	putBool(in, "enabled", m.Enabled, false)
	putString(in, "secret", m.Secret, false)
	_ = putStringSet(ctx, in, "resourceTypes", m.ResourceTypes, false)
	// Neither teamId nor allPublicTeams exists on WebhookUpdateInput.
	if !forUpdate {
		putString(in, "teamId", m.TeamID, false)
		putBool(in, "allPublicTeams", m.AllPublicTeams, false)
	}
	return in
}

func webhookSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a Linear webhook — the outbound HTTP callback Linear fires when the " +
			"subscribed resources change.\n\n" +
			"Scope it either to one team via `team_id` or to every public team via `all_public_teams`; both are " +
			"fixed at creation, since Linear's `webhookUpdate` takes neither.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the webhook.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "URL Linear posts events to.",
				Required:            true,
			},
			"label": schema.StringAttribute{
				MarkdownDescription: "Label identifying the webhook in the Linear UI.",
				Optional:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the webhook fires. Linear disables a webhook itself after repeated " +
					"delivery failures; setting this back to `true` re-enables it.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"resource_types": schema.SetAttribute{
				MarkdownDescription: "Resources the webhook subscribes to, e.g. `Issue`, `Comment`, `Project`, " +
					"`Cycle`, `IssueLabel`, `Reaction`.",
				Required:    true,
				ElementType: types.StringType,
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the webhook is scoped to. Leave unset together with " +
					"`all_public_teams` for a workspace-wide webhook. Changing it replaces the webhook.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"all_public_teams": schema.BoolAttribute{
				MarkdownDescription: "Whether the webhook covers every public team rather than a single one. " +
					"Changing it replaces the webhook.",
				Optional:      true,
				Computed:      true,
				Default:       booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"secret": schema.StringAttribute{
				MarkdownDescription: "Signing secret Linear uses for the `Linear-Signature` header, so the " +
					"receiver can verify a delivery came from Linear.\n\n" +
					"Write-only by design: it is sent but never read back, so it cannot surface through a refresh " +
					"or a provider log. It is still stored in state — keep the state encrypted.",
				Optional:  true,
				Sensitive: true,
			},
		},
	}
}
