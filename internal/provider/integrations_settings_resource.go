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

// This is the layer above the integration itself. Connecting Slack is an OAuth
// flow — not idempotent, not declarative, so no resource for it. Which events
// that connection then posts is plain configuration, and that is what this
// manages. Attach it to exactly one of a team, project, initiative or custom
// view; all four anchors are fixed at creation.
var integrationsSettingsEntity = entity{
	name: "integrationsSettings",
	fields: `id contextViewType
		slackIssueCreated slackIssueNewComment slackIssueAddedToTriage slackIssueAddedToView
		slackIssueStatusChangedAll slackIssueStatusChangedDone
		slackIssueSlaBreached slackIssueSlaHighRisk
		slackProjectUpdateCreated slackProjectUpdateCreatedToTeam slackProjectUpdateCreatedToWorkspace
		slackInitiativeUpdateCreated microsoftTeamsProjectUpdateCreated
		team { id } project { id } initiative { id }`,
}

// NewIntegrationsSettingsResource returns a new linear_integrations_settings resource.
func NewIntegrationsSettingsResource() resource.Resource {
	return &standardResource{
		entity:   integrationsSettingsEntity,
		typeName: "integrations_settings",
		kind:     "integrations settings",
		schema:   integrationsSettingsSchema,
		newModel: func() crudModel { return &integrationsSettingsModel{} },
	}
}

type integrationsSettingsAttributes struct {
	ID              string  `json:"id"`
	ContextViewType *string `json:"contextViewType"`

	SlackIssueCreated           *bool `json:"slackIssueCreated"`
	SlackIssueNewComment        *bool `json:"slackIssueNewComment"`
	SlackIssueAddedToTriage     *bool `json:"slackIssueAddedToTriage"`
	SlackIssueAddedToView       *bool `json:"slackIssueAddedToView"`
	SlackIssueStatusChangedAll  *bool `json:"slackIssueStatusChangedAll"`
	SlackIssueStatusChangedDone *bool `json:"slackIssueStatusChangedDone"`
	SlackIssueSlaBreached       *bool `json:"slackIssueSlaBreached"`
	SlackIssueSlaHighRisk       *bool `json:"slackIssueSlaHighRisk"`

	SlackProjectUpdateCreated            *bool `json:"slackProjectUpdateCreated"`
	SlackProjectUpdateCreatedToTeam      *bool `json:"slackProjectUpdateCreatedToTeam"`
	SlackProjectUpdateCreatedToWorkspace *bool `json:"slackProjectUpdateCreatedToWorkspace"`
	SlackInitiativeUpdateCreated         *bool `json:"slackInitiativeUpdateCreated"`
	MicrosoftTeamsProjectUpdateCreated   *bool `json:"microsoftTeamsProjectUpdateCreated"`

	Team       *ref `json:"team"`
	Project    *ref `json:"project"`
	Initiative *ref `json:"initiative"`
}

type integrationsSettingsModel struct {
	ID              types.String `tfsdk:"id"`
	TeamID          types.String `tfsdk:"team_id"`
	ProjectID       types.String `tfsdk:"project_id"`
	InitiativeID    types.String `tfsdk:"initiative_id"`
	CustomViewID    types.String `tfsdk:"custom_view_id"`
	ContextViewType types.String `tfsdk:"context_view_type"`

	SlackIssueCreated           types.Bool `tfsdk:"slack_issue_created"`
	SlackIssueNewComment        types.Bool `tfsdk:"slack_issue_new_comment"`
	SlackIssueAddedToTriage     types.Bool `tfsdk:"slack_issue_added_to_triage"`
	SlackIssueAddedToView       types.Bool `tfsdk:"slack_issue_added_to_view"`
	SlackIssueStatusChangedAll  types.Bool `tfsdk:"slack_issue_status_changed_all"`
	SlackIssueStatusChangedDone types.Bool `tfsdk:"slack_issue_status_changed_done"`
	SlackIssueSlaBreached       types.Bool `tfsdk:"slack_issue_sla_breached"`
	SlackIssueSlaHighRisk       types.Bool `tfsdk:"slack_issue_sla_high_risk"`

	SlackProjectUpdateCreated            types.Bool `tfsdk:"slack_project_update_created"`
	SlackProjectUpdateCreatedToTeam      types.Bool `tfsdk:"slack_project_update_created_to_team"`
	SlackProjectUpdateCreatedToWorkspace types.Bool `tfsdk:"slack_project_update_created_to_workspace"`
	SlackInitiativeUpdateCreated         types.Bool `tfsdk:"slack_initiative_update_created"`
	MicrosoftTeamsProjectUpdateCreated   types.Bool `tfsdk:"microsoft_teams_project_update_created"`
}

func (m *integrationsSettingsModel) id() string { return m.ID.ValueString() }

func (m *integrationsSettingsModel) decode(_ context.Context, raw json.RawMessage) error {
	var a integrationsSettingsAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.ContextViewType = types.StringPointerValue(a.ContextViewType)

	m.SlackIssueCreated = types.BoolPointerValue(a.SlackIssueCreated)
	m.SlackIssueNewComment = types.BoolPointerValue(a.SlackIssueNewComment)
	m.SlackIssueAddedToTriage = types.BoolPointerValue(a.SlackIssueAddedToTriage)
	m.SlackIssueAddedToView = types.BoolPointerValue(a.SlackIssueAddedToView)
	m.SlackIssueStatusChangedAll = types.BoolPointerValue(a.SlackIssueStatusChangedAll)
	m.SlackIssueStatusChangedDone = types.BoolPointerValue(a.SlackIssueStatusChangedDone)
	m.SlackIssueSlaBreached = types.BoolPointerValue(a.SlackIssueSlaBreached)
	m.SlackIssueSlaHighRisk = types.BoolPointerValue(a.SlackIssueSlaHighRisk)

	m.SlackProjectUpdateCreated = types.BoolPointerValue(a.SlackProjectUpdateCreated)
	m.SlackProjectUpdateCreatedToTeam = types.BoolPointerValue(a.SlackProjectUpdateCreatedToTeam)
	m.SlackProjectUpdateCreatedToWorkspace = types.BoolPointerValue(a.SlackProjectUpdateCreatedToWorkspace)
	m.SlackInitiativeUpdateCreated = types.BoolPointerValue(a.SlackInitiativeUpdateCreated)
	m.MicrosoftTeamsProjectUpdateCreated = types.BoolPointerValue(a.MicrosoftTeamsProjectUpdateCreated)

	m.TeamID = refID(a.Team)
	m.ProjectID = refID(a.Project)
	m.InitiativeID = refID(a.Initiative)
	// custom_view_id has no read path — IntegrationsSettings does not expose the
	// view it was attached to — so it keeps whatever the configuration declared.
	return nil
}

func (m *integrationsSettingsModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{}
	putBool(in, "slackIssueCreated", m.SlackIssueCreated, false)
	putBool(in, "slackIssueNewComment", m.SlackIssueNewComment, false)
	putBool(in, "slackIssueAddedToTriage", m.SlackIssueAddedToTriage, false)
	putBool(in, "slackIssueAddedToView", m.SlackIssueAddedToView, false)
	putBool(in, "slackIssueStatusChangedAll", m.SlackIssueStatusChangedAll, false)
	putBool(in, "slackIssueStatusChangedDone", m.SlackIssueStatusChangedDone, false)
	putBool(in, "slackIssueSlaBreached", m.SlackIssueSlaBreached, false)
	putBool(in, "slackIssueSlaHighRisk", m.SlackIssueSlaHighRisk, false)
	putBool(in, "slackProjectUpdateCreated", m.SlackProjectUpdateCreated, false)
	putBool(in, "slackProjectUpdateCreatedToTeam", m.SlackProjectUpdateCreatedToTeam, false)
	putBool(in, "slackProjectUpdateCreatedToWorkspace", m.SlackProjectUpdateCreatedToWorkspace, false)
	putBool(in, "slackInitiativeUpdateCreated", m.SlackInitiativeUpdateCreated, false)
	putBool(in, "microsoftTeamsProjectUpdateCreated", m.MicrosoftTeamsProjectUpdateCreated, false)

	// The anchors and the view type only exist on the create input.
	if !forUpdate {
		putString(in, "teamId", m.TeamID, false)
		putString(in, "projectId", m.ProjectID, false)
		putString(in, "initiativeId", m.InitiativeID, false)
		putString(in, "customViewId", m.CustomViewID, false)
		putString(in, "contextViewType", m.ContextViewType, false)
	}
	return in
}

func integrationsSettingsSchema() schema.Schema {
	// The anchors are fixed at creation and Linear reports them back, so they are
	// Optional + Computed: an anchor the configuration omits keeps whatever the
	// resource was created against instead of planning a replace against null.
	replaceString := func(desc string) schema.Attribute {
		return schema.StringAttribute{
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers:       keepString(stringplanmodifier.RequiresReplace()),
		}
	}
	// custom_view_id is the one anchor Linear never reports back, so it cannot be
	// Computed — the value Terraform planned is not one that comes back, and the
	// apply would fail on the mismatch.
	writeOnlyReplaceString := func(desc string) schema.Attribute {
		return schema.StringAttribute{
			MarkdownDescription: desc,
			Optional:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		}
	}
	notify := func(desc string) schema.Attribute {
		return schema.BoolAttribute{
			MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: keepBool(),
		}
	}

	return schema.Schema{
		MarkdownDescription: "Manages which events a Linear integration posts — the notification matrix behind " +
			"an already-connected Slack or Microsoft Teams integration.\n\n" +
			"Connecting the integration itself is an OAuth flow and not something a provider can manage " +
			"declaratively; this resource configures what an existing connection does. Attach it to exactly one " +
			"of a team, project, initiative or custom view — all four are fixed at creation, since Linear's " +
			"update mutation takes only the toggles.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the integrations settings.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id":       replaceString("UUID of the team these settings belong to."),
			"project_id":    replaceString("UUID of the project these settings belong to."),
			"initiative_id": replaceString("UUID of the initiative these settings belong to."),
			"custom_view_id": writeOnlyReplaceString("UUID of the `linear_custom_view` these settings belong to. " +
				"**Write-only**: Linear does not report which view the settings belong to, so drift in this " +
				"attribute cannot be detected."),
			"context_view_type": replaceString("Which surface the settings apply to when attached to a project " +
				"or initiative, e.g. `project` or `initiative`."),

			"slack_issue_created":            notify("Post to Slack when an issue is created."),
			"slack_issue_new_comment":        notify("Post to Slack when an issue gets a comment."),
			"slack_issue_added_to_triage":    notify("Post to Slack when an issue lands in triage."),
			"slack_issue_added_to_view":      notify("Post to Slack when an issue enters the attached view."),
			"slack_issue_status_changed_all": notify("Post to Slack on every issue status change."),
			"slack_issue_status_changed_done": notify("Post to Slack when an issue is completed. Redundant when " +
				"`slack_issue_status_changed_all` is on."),
			"slack_issue_sla_breached":  notify("Post to Slack when an issue breaches its SLA."),
			"slack_issue_sla_high_risk": notify("Post to Slack when an issue is at risk of breaching its SLA."),

			"slack_project_update_created":              notify("Post to Slack when a project update is written."),
			"slack_project_update_created_to_team":      notify("Post project updates to the team's Slack channel."),
			"slack_project_update_created_to_workspace": notify("Post project updates to the workspace Slack channel."),
			"slack_initiative_update_created":           notify("Post to Slack when an initiative update is written."),
			"microsoft_teams_project_update_created":    notify("Post to Microsoft Teams when a project update is written."),
		},
	}
}
