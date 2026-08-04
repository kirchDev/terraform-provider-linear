package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// The workspace is a singleton: an API key is scoped to exactly one, there is no
// organizationCreate, and organizationDelete is not something a provider should
// ever reach for. So this resource is manage-not-create — Create adopts the
// existing workspace by applying the configuration to it, and Delete only drops
// the resource from state.
//
// Every attribute is Optional + Computed, without exception. With ~50 attributes
// on one resource the alternative is an apply that silently resets whatever the
// config did not mention, across an entire workspace.

const organizationFields = `id name urlKey logoUrl
	gitBranchFormat gitLinkbackMessagesEnabled gitLinkbackDescriptionsEnabled gitPublicLinkbackMessagesEnabled
	workingDays fiscalYearStartMonth
	roadmapEnabled releasesEnabled feedEnabled customersEnabled generatedUpdatesEnabled
	defaultHomeView defaultHomeViewTargetId defaultFeedSummarySchedule
	pullRequestIssueMode pullRequestTourEnabled
	allowedFileUploadContentTypes hipaaComplianceEnabled
	projectUpdateReminderFrequencyInWeeks projectUpdateRemindersDay projectUpdateRemindersHour
	initiativeUpdateReminderFrequencyInWeeks initiativeUpdateRemindersDay initiativeUpdateRemindersHour
	aiAddonEnabled aiTelemetryEnabled agentAutomationEnabled
	aiThreadSummariesEnabled aiDiscussionSummariesEnabled
	codeIntelligenceEnabled codeIntelligenceRepository
	codingAgentEnabled linearAgentEnabled restrictAgentInvocationToMembers
	slackProjectChannelsEnabled slackProjectChannelPrefix slackAutoCreateProjectChannel
	slackProjectChannelIntegration { id }
	securitySettings authSettings themeSettings codingAgentSettings linearAgentSettings
	customersConfiguration
	ipRestrictions { range type enabled description }`

var (
	_ resource.Resource                = (*workspaceSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*workspaceSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*workspaceSettingsResource)(nil)
)

// NewWorkspaceSettingsResource returns a new linear_workspace_settings resource.
func NewWorkspaceSettingsResource() resource.Resource {
	return &workspaceSettingsResource{}
}

type workspaceSettingsResource struct {
	client *client.Client
}

type organizationAttributes struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	URLKey  string  `json:"urlKey"`
	LogoURL *string `json:"logoUrl"`

	GitBranchFormat                  *string `json:"gitBranchFormat"`
	GitLinkbackMessagesEnabled       bool    `json:"gitLinkbackMessagesEnabled"`
	GitLinkbackDescriptionsEnabled   bool    `json:"gitLinkbackDescriptionsEnabled"`
	GitPublicLinkbackMessagesEnabled bool    `json:"gitPublicLinkbackMessagesEnabled"`

	WorkingDays          []float64 `json:"workingDays"`
	FiscalYearStartMonth float64   `json:"fiscalYearStartMonth"`

	RoadmapEnabled          bool `json:"roadmapEnabled"`
	ReleasesEnabled         bool `json:"releasesEnabled"`
	FeedEnabled             bool `json:"feedEnabled"`
	CustomersEnabled        bool `json:"customersEnabled"`
	GeneratedUpdatesEnabled bool `json:"generatedUpdatesEnabled"`

	DefaultHomeView            *string `json:"defaultHomeView"`
	DefaultHomeViewTargetID    *string `json:"defaultHomeViewTargetId"`
	DefaultFeedSummarySchedule *string `json:"defaultFeedSummarySchedule"`

	PullRequestIssueMode   string `json:"pullRequestIssueMode"`
	PullRequestTourEnabled bool   `json:"pullRequestTourEnabled"`

	AllowedFileUploadContentTypes []string `json:"allowedFileUploadContentTypes"`
	HIPAAComplianceEnabled        bool     `json:"hipaaComplianceEnabled"`

	ProjectUpdateReminderFrequencyInWeeks    *float64 `json:"projectUpdateReminderFrequencyInWeeks"`
	ProjectUpdateRemindersDay                string   `json:"projectUpdateRemindersDay"`
	ProjectUpdateRemindersHour               float64  `json:"projectUpdateRemindersHour"`
	InitiativeUpdateReminderFrequencyInWeeks *float64 `json:"initiativeUpdateReminderFrequencyInWeeks"`
	InitiativeUpdateRemindersDay             string   `json:"initiativeUpdateRemindersDay"`
	InitiativeUpdateRemindersHour            float64  `json:"initiativeUpdateRemindersHour"`

	AIAddonEnabled               bool `json:"aiAddonEnabled"`
	AITelemetryEnabled           bool `json:"aiTelemetryEnabled"`
	AgentAutomationEnabled       bool `json:"agentAutomationEnabled"`
	AIThreadSummariesEnabled     bool `json:"aiThreadSummariesEnabled"`
	AIDiscussionSummariesEnabled bool `json:"aiDiscussionSummariesEnabled"`

	CodeIntelligenceEnabled          bool    `json:"codeIntelligenceEnabled"`
	CodeIntelligenceRepository       *string `json:"codeIntelligenceRepository"`
	CodingAgentEnabled               bool    `json:"codingAgentEnabled"`
	LinearAgentEnabled               bool    `json:"linearAgentEnabled"`
	RestrictAgentInvocationToMembers *bool   `json:"restrictAgentInvocationToMembers"`

	SlackProjectChannelsEnabled    bool   `json:"slackProjectChannelsEnabled"`
	SlackProjectChannelPrefix      string `json:"slackProjectChannelPrefix"`
	SlackAutoCreateProjectChannel  bool   `json:"slackAutoCreateProjectChannel"`
	SlackProjectChannelIntegration *ref   `json:"slackProjectChannelIntegration"`

	SecuritySettings       json.RawMessage `json:"securitySettings"`
	AuthSettings           json.RawMessage `json:"authSettings"`
	ThemeSettings          json.RawMessage `json:"themeSettings"`
	CodingAgentSettings    json.RawMessage `json:"codingAgentSettings"`
	LinearAgentSettings    json.RawMessage `json:"linearAgentSettings"`
	CustomersConfiguration json.RawMessage `json:"customersConfiguration"`

	IPRestrictions []organizationIPRestriction `json:"ipRestrictions"`
}

// organizationIPRestriction is one entry of Organization.ipRestrictions. Every
// other settings blob on the workspace is a JSONObject scalar that travels as
// raw JSON; this one is a typed list, so the query has to select its subfields
// and the provider has to put them back together itself.
//
// Description is a pointer with omitempty for a reason: GraphQL answers with
// every subfield the document selected, so a restriction carrying no
// description comes back as `"description": null`, while the configuration that
// wrote it simply left the key out. Keeping the null would make those two
// unequal — jsontypes.Normalized compares the parsed documents, and a key
// holding null is not an absent key — and end the apply that wrote it in
// "provider produced inconsistent result after apply".
type organizationIPRestriction struct {
	Range       string  `json:"range"`
	Type        string  `json:"type"`
	Enabled     bool    `json:"enabled"`
	Description *string `json:"description,omitempty"`
}

// ipRestrictionsAttr re-serialises the IP restriction list into the
// ip_restrictions_json attribute. jsonAttr cannot do it: that hands a JSON
// scalar through exactly as Linear serialised it, which is right for the
// JSONObject settings and wrong here, where the nulls have to go first.
func ipRestrictionsAttr(in []organizationIPRestriction) (jsontypes.Normalized, error) {
	if in == nil {
		return jsontypes.NewNormalizedNull(), nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return jsontypes.NewNormalizedNull(), err
	}
	return jsontypes.NewNormalizedValue(string(raw)), nil
}

type workspaceSettingsModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	URLKey  types.String `tfsdk:"url_key"`
	LogoURL types.String `tfsdk:"logo_url"`

	GitBranchFormat                  types.String `tfsdk:"git_branch_format"`
	GitLinkbackMessagesEnabled       types.Bool   `tfsdk:"git_linkback_messages_enabled"`
	GitLinkbackDescriptionsEnabled   types.Bool   `tfsdk:"git_linkback_descriptions_enabled"`
	GitPublicLinkbackMessagesEnabled types.Bool   `tfsdk:"git_public_linkback_messages_enabled"`

	WorkingDays          types.List    `tfsdk:"working_days"`
	FiscalYearStartMonth types.Float64 `tfsdk:"fiscal_year_start_month"`

	RoadmapEnabled          types.Bool `tfsdk:"roadmap_enabled"`
	ReleasesEnabled         types.Bool `tfsdk:"releases_enabled"`
	FeedEnabled             types.Bool `tfsdk:"feed_enabled"`
	CustomersEnabled        types.Bool `tfsdk:"customers_enabled"`
	GeneratedUpdatesEnabled types.Bool `tfsdk:"generated_updates_enabled"`

	DefaultHomeView            types.String `tfsdk:"default_home_view"`
	DefaultHomeViewTargetID    types.String `tfsdk:"default_home_view_target_id"`
	DefaultFeedSummarySchedule types.String `tfsdk:"default_feed_summary_schedule"`

	PullRequestIssueMode   types.String `tfsdk:"pull_request_issue_mode"`
	PullRequestTourEnabled types.Bool   `tfsdk:"pull_request_tour_enabled"`

	AllowedFileUploadContentTypes types.List `tfsdk:"allowed_file_upload_content_types"`
	HIPAAComplianceEnabled        types.Bool `tfsdk:"hipaa_compliance_enabled"`

	ProjectUpdateReminderFrequencyInWeeks    types.Float64 `tfsdk:"project_update_reminder_frequency_in_weeks"`
	ProjectUpdateRemindersDay                types.String  `tfsdk:"project_update_reminders_day"`
	ProjectUpdateRemindersHour               types.Float64 `tfsdk:"project_update_reminders_hour"`
	InitiativeUpdateReminderFrequencyInWeeks types.Float64 `tfsdk:"initiative_update_reminder_frequency_in_weeks"`
	InitiativeUpdateRemindersDay             types.String  `tfsdk:"initiative_update_reminders_day"`
	InitiativeUpdateRemindersHour            types.Float64 `tfsdk:"initiative_update_reminders_hour"`

	AIAddonEnabled               types.Bool `tfsdk:"ai_addon_enabled"`
	AITelemetryEnabled           types.Bool `tfsdk:"ai_telemetry_enabled"`
	AgentAutomationEnabled       types.Bool `tfsdk:"agent_automation_enabled"`
	AIThreadSummariesEnabled     types.Bool `tfsdk:"ai_thread_summaries_enabled"`
	AIDiscussionSummariesEnabled types.Bool `tfsdk:"ai_discussion_summaries_enabled"`

	CodeIntelligenceEnabled          types.Bool   `tfsdk:"code_intelligence_enabled"`
	CodeIntelligenceRepository       types.String `tfsdk:"code_intelligence_repository"`
	CodingAgentEnabled               types.Bool   `tfsdk:"coding_agent_enabled"`
	LinearAgentEnabled               types.Bool   `tfsdk:"linear_agent_enabled"`
	RestrictAgentInvocationToMembers types.Bool   `tfsdk:"restrict_agent_invocation_to_members"`

	SlackProjectChannelsEnabled      types.Bool   `tfsdk:"slack_project_channels_enabled"`
	SlackProjectChannelPrefix        types.String `tfsdk:"slack_project_channel_prefix"`
	SlackAutoCreateProjectChannel    types.Bool   `tfsdk:"slack_auto_create_project_channel"`
	SlackProjectChannelIntegrationID types.String `tfsdk:"slack_project_channel_integration_id"`

	SecuritySettingsJSON       jsontypes.Normalized `tfsdk:"security_settings_json"`
	AuthSettingsJSON           jsontypes.Normalized `tfsdk:"auth_settings_json"`
	ThemeSettingsJSON          jsontypes.Normalized `tfsdk:"theme_settings_json"`
	CodingAgentSettingsJSON    jsontypes.Normalized `tfsdk:"coding_agent_settings_json"`
	LinearAgentSettingsJSON    jsontypes.Normalized `tfsdk:"linear_agent_settings_json"`
	CustomersConfigurationJSON jsontypes.Normalized `tfsdk:"customers_configuration_json"`
	IPRestrictionsJSON         jsontypes.Normalized `tfsdk:"ip_restrictions_json"`

	// Write-only: accepted on OrganizationUpdateInput but absent from the
	// Organization type, so there is nothing to refresh them against.
	SLAEnabled                 types.Bool `tfsdk:"sla_enabled"`
	OAuthAppReview             types.Bool `tfsdk:"oauth_app_review"`
	ReducedPersonalInformation types.Bool `tfsdk:"reduced_personal_information"`
}

func (m *workspaceSettingsModel) decode(ctx context.Context, raw json.RawMessage) error {
	var a organizationAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}

	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.URLKey = types.StringValue(a.URLKey)
	m.LogoURL = types.StringPointerValue(a.LogoURL)

	m.GitBranchFormat = types.StringPointerValue(a.GitBranchFormat)
	m.GitLinkbackMessagesEnabled = types.BoolValue(a.GitLinkbackMessagesEnabled)
	m.GitLinkbackDescriptionsEnabled = types.BoolValue(a.GitLinkbackDescriptionsEnabled)
	m.GitPublicLinkbackMessagesEnabled = types.BoolValue(a.GitPublicLinkbackMessagesEnabled)

	days, d := types.ListValueFrom(ctx, types.Float64Type, orEmptyFloats(a.WorkingDays))
	if d.HasError() {
		return errBuildingList("working_days")
	}
	m.WorkingDays = days
	m.FiscalYearStartMonth = types.Float64Value(a.FiscalYearStartMonth)

	m.RoadmapEnabled = types.BoolValue(a.RoadmapEnabled)
	m.ReleasesEnabled = types.BoolValue(a.ReleasesEnabled)
	m.FeedEnabled = types.BoolValue(a.FeedEnabled)
	m.CustomersEnabled = types.BoolValue(a.CustomersEnabled)
	m.GeneratedUpdatesEnabled = types.BoolValue(a.GeneratedUpdatesEnabled)

	m.DefaultHomeView = types.StringPointerValue(a.DefaultHomeView)
	m.DefaultHomeViewTargetID = types.StringPointerValue(a.DefaultHomeViewTargetID)
	m.DefaultFeedSummarySchedule = types.StringPointerValue(a.DefaultFeedSummarySchedule)

	m.PullRequestIssueMode = types.StringValue(a.PullRequestIssueMode)
	m.PullRequestTourEnabled = types.BoolValue(a.PullRequestTourEnabled)

	types_, err := listOfStrings(ctx, a.AllowedFileUploadContentTypes)
	if err != nil {
		return err
	}
	m.AllowedFileUploadContentTypes = types_
	m.HIPAAComplianceEnabled = types.BoolValue(a.HIPAAComplianceEnabled)

	m.ProjectUpdateReminderFrequencyInWeeks = types.Float64PointerValue(a.ProjectUpdateReminderFrequencyInWeeks)
	m.ProjectUpdateRemindersDay = stringOrNull(a.ProjectUpdateRemindersDay)
	m.ProjectUpdateRemindersHour = types.Float64Value(a.ProjectUpdateRemindersHour)
	m.InitiativeUpdateReminderFrequencyInWeeks = types.Float64PointerValue(a.InitiativeUpdateReminderFrequencyInWeeks)
	m.InitiativeUpdateRemindersDay = stringOrNull(a.InitiativeUpdateRemindersDay)
	m.InitiativeUpdateRemindersHour = types.Float64Value(a.InitiativeUpdateRemindersHour)

	m.AIAddonEnabled = types.BoolValue(a.AIAddonEnabled)
	m.AITelemetryEnabled = types.BoolValue(a.AITelemetryEnabled)
	m.AgentAutomationEnabled = types.BoolValue(a.AgentAutomationEnabled)
	m.AIThreadSummariesEnabled = types.BoolValue(a.AIThreadSummariesEnabled)
	m.AIDiscussionSummariesEnabled = types.BoolValue(a.AIDiscussionSummariesEnabled)

	m.CodeIntelligenceEnabled = types.BoolValue(a.CodeIntelligenceEnabled)
	m.CodeIntelligenceRepository = types.StringPointerValue(a.CodeIntelligenceRepository)
	m.CodingAgentEnabled = types.BoolValue(a.CodingAgentEnabled)
	m.LinearAgentEnabled = types.BoolValue(a.LinearAgentEnabled)
	m.RestrictAgentInvocationToMembers = types.BoolPointerValue(a.RestrictAgentInvocationToMembers)

	m.SlackProjectChannelsEnabled = types.BoolValue(a.SlackProjectChannelsEnabled)
	m.SlackProjectChannelPrefix = stringOrNull(a.SlackProjectChannelPrefix)
	m.SlackAutoCreateProjectChannel = types.BoolValue(a.SlackAutoCreateProjectChannel)
	m.SlackProjectChannelIntegrationID = refID(a.SlackProjectChannelIntegration)

	m.SecuritySettingsJSON = jsonAttr(a.SecuritySettings)
	m.AuthSettingsJSON = jsonAttr(a.AuthSettings)
	m.ThemeSettingsJSON = jsonAttr(a.ThemeSettings)
	m.CodingAgentSettingsJSON = jsonAttr(a.CodingAgentSettings)
	m.LinearAgentSettingsJSON = jsonAttr(a.LinearAgentSettings)
	m.CustomersConfigurationJSON = jsonAttr(a.CustomersConfiguration)

	restrictions, err := ipRestrictionsAttr(a.IPRestrictions)
	if err != nil {
		return err
	}
	m.IPRestrictionsJSON = restrictions
	return nil
}

// input builds the OrganizationUpdateInput. Nothing is ever cleared: an
// attribute the config omits keeps its live value, which is the whole point of
// every attribute being Optional + Computed here.
func (m *workspaceSettingsModel) input(ctx context.Context) (map[string]any, error) {
	in := map[string]any{}

	putString(in, "name", m.Name, false)
	putString(in, "urlKey", m.URLKey, false)
	putString(in, "logoUrl", m.LogoURL, false)

	putString(in, "gitBranchFormat", m.GitBranchFormat, false)
	putBool(in, "gitLinkbackMessagesEnabled", m.GitLinkbackMessagesEnabled, false)
	putBool(in, "gitLinkbackDescriptionsEnabled", m.GitLinkbackDescriptionsEnabled, false)
	putBool(in, "gitPublicLinkbackMessagesEnabled", m.GitPublicLinkbackMessagesEnabled, false)

	if err := putFloatList(ctx, in, "workingDays", m.WorkingDays); err != nil {
		return nil, err
	}
	putFloat(in, "fiscalYearStartMonth", m.FiscalYearStartMonth, false)

	putBool(in, "roadmapEnabled", m.RoadmapEnabled, false)
	putBool(in, "feedEnabled", m.FeedEnabled, false)
	putBool(in, "customersEnabled", m.CustomersEnabled, false)
	putBool(in, "generatedUpdatesEnabled", m.GeneratedUpdatesEnabled, false)

	putString(in, "defaultHomeView", m.DefaultHomeView, false)
	putString(in, "defaultHomeViewTargetId", m.DefaultHomeViewTargetID, false)
	putString(in, "defaultFeedSummarySchedule", m.DefaultFeedSummarySchedule, false)

	putString(in, "pullRequestIssueMode", m.PullRequestIssueMode, false)
	putBool(in, "pullRequestTourEnabled", m.PullRequestTourEnabled, false)

	if err := putStringList(ctx, in, "allowedFileUploadContentTypes", m.AllowedFileUploadContentTypes, false); err != nil {
		return nil, err
	}
	putBool(in, "hipaaComplianceEnabled", m.HIPAAComplianceEnabled, false)

	putFloat(in, "projectUpdateReminderFrequencyInWeeks", m.ProjectUpdateReminderFrequencyInWeeks, false)
	putString(in, "projectUpdateRemindersDay", m.ProjectUpdateRemindersDay, false)
	putFloat(in, "projectUpdateRemindersHour", m.ProjectUpdateRemindersHour, false)
	putFloat(in, "initiativeUpdateReminderFrequencyInWeeks", m.InitiativeUpdateReminderFrequencyInWeeks, false)
	putString(in, "initiativeUpdateRemindersDay", m.InitiativeUpdateRemindersDay, false)
	putFloat(in, "initiativeUpdateRemindersHour", m.InitiativeUpdateRemindersHour, false)

	putBool(in, "aiAddonEnabled", m.AIAddonEnabled, false)
	putBool(in, "aiTelemetryEnabled", m.AITelemetryEnabled, false)
	putBool(in, "agentAutomationEnabled", m.AgentAutomationEnabled, false)
	putBool(in, "aiThreadSummariesEnabled", m.AIThreadSummariesEnabled, false)
	putBool(in, "aiDiscussionSummariesEnabled", m.AIDiscussionSummariesEnabled, false)

	putBool(in, "codeIntelligenceEnabled", m.CodeIntelligenceEnabled, false)
	putString(in, "codeIntelligenceRepository", m.CodeIntelligenceRepository, false)
	putBool(in, "codingAgentEnabled", m.CodingAgentEnabled, false)
	putBool(in, "linearAgentEnabled", m.LinearAgentEnabled, false)
	putBool(in, "restrictAgentInvocationToMembers", m.RestrictAgentInvocationToMembers, false)

	putBool(in, "slackProjectChannelsEnabled", m.SlackProjectChannelsEnabled, false)
	putString(in, "slackProjectChannelPrefix", m.SlackProjectChannelPrefix, false)
	putBool(in, "slackAutoCreateProjectChannel", m.SlackAutoCreateProjectChannel, false)
	putString(in, "slackProjectChannelIntegrationId", m.SlackProjectChannelIntegrationID, false)

	putBool(in, "slaEnabled", m.SLAEnabled, false)
	putBool(in, "oauthAppReview", m.OAuthAppReview, false)
	putBool(in, "reducedPersonalInformation", m.ReducedPersonalInformation, false)

	for _, f := range []struct {
		key   string
		value jsontypes.Normalized
	}{
		{"securitySettings", m.SecuritySettingsJSON},
		{"authSettings", m.AuthSettingsJSON},
		{"themeSettings", m.ThemeSettingsJSON},
		{"codingAgentSettings", m.CodingAgentSettingsJSON},
		{"linearAgentSettings", m.LinearAgentSettingsJSON},
		{"customersConfiguration", m.CustomersConfigurationJSON},
		{"ipRestrictions", m.IPRestrictionsJSON},
	} {
		if err := putJSON(in, f.key, f.value, false); err != nil {
			return nil, err
		}
	}
	return in, nil
}

func (r *workspaceSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace_settings"
}

func (r *workspaceSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optBool := func(desc string) schema.Attribute {
		return schema.BoolAttribute{MarkdownDescription: desc, Optional: true, Computed: true}
	}
	optString := func(desc string) schema.Attribute {
		return schema.StringAttribute{MarkdownDescription: desc, Optional: true, Computed: true}
	}
	optFloat := func(desc string) schema.Attribute {
		return schema.Float64Attribute{MarkdownDescription: desc, Optional: true, Computed: true}
	}
	optJSON := func(desc string) schema.Attribute {
		return schema.StringAttribute{
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			CustomType:          jsontypes.NormalizedType{},
		}
	}
	writeOnlyBool := func(desc string) schema.Attribute {
		return schema.BoolAttribute{MarkdownDescription: desc, Optional: true}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the settings of the Linear workspace the API key belongs to.\n\n" +
			"This is a singleton and **manage-not-create**: applying it adopts the existing workspace rather than " +
			"creating one, and destroying the resource only drops it from state — no workspace is deleted and no " +
			"setting is reset. Declare it at most once per provider instance.\n\n" +
			"Every attribute is optional and computed: whatever the configuration leaves out keeps its live value. " +
			"With this many settings on one resource, the alternative would be an apply that silently resets an " +
			"entire workspace.\n\n" +
			"Member invitation, team-creation and label-management restrictions moved into `security_settings_json` " +
			"on Linear's side; they are exposed there and nowhere else, so there is exactly one way in.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the workspace.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":     optString("Display name of the workspace."),
			"url_key":  optString("URL slug of the workspace — the `<key>` in `linear.app/<key>`. Changing it breaks existing links."),
			"logo_url": optString("URL of the workspace logo."),

			"git_branch_format": optString("Template Linear generates git branch names from, e.g. " +
				"`{teamKey}/{issueIdentifier}-{issueTitle}`."),
			"git_linkback_messages_enabled":        optBool("Whether Linear comments a linkback on private repositories."),
			"git_linkback_descriptions_enabled":    optBool("Whether Linear adds a linkback to pull request descriptions."),
			"git_public_linkback_messages_enabled": optBool("Whether Linear comments a linkback on public repositories."),

			"working_days": schema.ListAttribute{
				MarkdownDescription: "Weekdays the workspace works on, `0` being Sunday. Drives cycle and SLA maths.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.Float64Type,
			},
			"fiscal_year_start_month": optFloat("Month the fiscal year starts in, `0` being January."),

			"roadmap_enabled": optBool("Whether Initiatives are available in the workspace. Linear renamed the " +
				"feature to Initiatives; its API still calls the field `roadmapEnabled`, the name it shipped " +
				"under. This is the workspace-level toggle — `linear_team.initiatives_enabled` is the " +
				"per-team one."),
			"feed_enabled":              optBool("Whether the workspace feed is available."),
			"customers_enabled":         optBool("Whether Linear Customers is enabled — the prerequisite for `linear_customer_status` and `linear_customer_tier`."),
			"generated_updates_enabled": optBool("Whether Linear generates project and initiative updates automatically."),
			"releases_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether releases are enabled. **Read-only** — Linear reports it but " +
					"`organizationUpdate` takes no matching field.",
				Computed: true,
			},

			"default_home_view":           optString("View new members land on, e.g. `activeIssues`, `myIssues`, `inbox`."),
			"default_home_view_target_id": optString("UUID of the target when `default_home_view` points at a specific view."),
			"default_feed_summary_schedule": optString("How often the feed summary is generated, e.g. `daily`, " +
				"`weekly`, `never`."),

			"pull_request_issue_mode": optString("How Linear turns pull requests into issues — e.g. `disabled`, " +
				"`enabled`, `readOnly`."),
			"pull_request_tour_enabled": optBool("Whether the pull request tour is shown."),

			"allowed_file_upload_content_types": schema.ListAttribute{
				MarkdownDescription: "MIME types members may upload. An empty list allows everything.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
			"hipaa_compliance_enabled": optBool("Whether HIPAA compliance mode is on."),

			"project_update_reminder_frequency_in_weeks":    optFloat("How many weeks between project update reminders."),
			"project_update_reminders_day":                  optString("Weekday project update reminders go out, e.g. `Friday`."),
			"project_update_reminders_hour":                 optFloat("Hour of day project update reminders go out, 0–23."),
			"initiative_update_reminder_frequency_in_weeks": optFloat("How many weeks between initiative update reminders."),
			"initiative_update_reminders_day":               optString("Weekday initiative update reminders go out."),
			"initiative_update_reminders_hour":              optFloat("Hour of day initiative update reminders go out, 0–23."),

			"ai_addon_enabled":                optBool("Whether the Linear AI add-on is enabled for the workspace."),
			"ai_telemetry_enabled":            optBool("Whether AI telemetry is shared with Linear."),
			"agent_automation_enabled":        optBool("Whether agents may run automations."),
			"ai_thread_summaries_enabled":     optBool("Whether Linear summarises long comment threads."),
			"ai_discussion_summaries_enabled": optBool("Whether Linear summarises discussions."),

			"code_intelligence_enabled":    optBool("Whether code intelligence is enabled."),
			"code_intelligence_repository": optString("Repository code intelligence indexes, as `owner/name`."),
			"coding_agent_enabled":         optBool("Whether the coding agent is enabled."),
			"linear_agent_enabled":         optBool("Whether the Linear agent is enabled."),
			"restrict_agent_invocation_to_members": optBool("Whether only workspace members — not guests — may " +
				"invoke agents."),

			"slack_project_channels_enabled":       optBool("Whether Linear manages Slack channels for projects."),
			"slack_project_channel_prefix":         optString("Prefix Linear gives generated Slack project channels."),
			"slack_auto_create_project_channel":    optBool("Whether a Slack channel is created for every new project."),
			"slack_project_channel_integration_id": optString("UUID of the Slack integration project channels are created through."),

			"security_settings_json": optJSON("Workspace security settings as a JSON object — this is where " +
				"member invitation, team creation and label management restrictions live now, e.g. " +
				"`jsonencode({ allowMembersToInvite = false, restrictTeamCreationToAdmins = true })`. " +
				"Compared semantically."),
			"auth_settings_json":           optJSON("Authentication settings as a JSON object — SAML, SCIM and allowed auth services."),
			"theme_settings_json":          optJSON("Workspace theme as a JSON object."),
			"coding_agent_settings_json":   optJSON("Coding agent settings as a JSON object."),
			"linear_agent_settings_json":   optJSON("Linear agent settings as a JSON object."),
			"customers_configuration_json": optJSON("Linear Customers configuration as a JSON object."),
			"ip_restrictions_json": optJSON("IP restrictions as a JSON array, e.g. " +
				"`jsonencode([{ range = \"203.0.113.0/24\", type = \"allow\", enabled = true }])`. Each entry " +
				"takes `range`, `type` and `enabled`, plus an optional `description`. Compared semantically, and " +
				"an entry without a description is read back without the key."),

			// Accepted by organizationUpdate but absent from the Organization type,
			// so there is nothing to refresh them against. Optional, never Computed:
			// state keeps what the config declared and drift goes unnoticed.
			"sla_enabled": writeOnlyBool("Whether SLAs are enabled. **Write-only** — Linear accepts it but does " +
				"not report it back, so drift in this attribute cannot be detected."),
			"oauth_app_review": writeOnlyBool("Whether OAuth applications need admin review before members may " +
				"install them. **Write-only**, for the same reason as `sla_enabled`."),
			"reduced_personal_information": writeOnlyBool("Whether Linear minimises the personal information it " +
				"stores. **Write-only**, for the same reason as `sla_enabled`."),
		},
	}
}

func (r *workspaceSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := resourceClient(req, resp); ok {
		r.client = c
	}
}

// Create adopts the workspace the API key belongs to rather than creating one.
func (r *workspaceSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan workspaceSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *workspaceSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state workspaceSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := r.readOrganization(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Linear workspace settings", err.Error())
		return
	}
	if err := state.decode(ctx, raw); err != nil {
		resp.Diagnostics.AddError("Unable to read Linear workspace settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *workspaceSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan workspaceSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics, &resp.State)
}

// Delete is a no-op beyond dropping the resource from state — destroying a
// workspace is not something this provider offers, and resetting ~50 settings to
// some notion of "default" would be worse than leaving them.
func (r *workspaceSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState adopts the workspace regardless of the id given, since the API key
// already determines which workspace this is. `terraform import
// linear_workspace_settings.this workspace` reads sensibly enough.
func (r *workspaceSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// apply pushes the configuration and writes back what the API returned.
func (r *workspaceSettingsResource) apply(ctx context.Context, plan *workspaceSettingsModel, diags *diag.Diagnostics, state *tfsdk.State) {
	in, err := plan.input(ctx)
	if err != nil {
		diags.AddError("Unable to build Linear workspace settings input", err.Error())
		return
	}

	doc := "mutation organizationUpdate($input: OrganizationUpdateInput!) {\n" +
		"  organizationUpdate(input: $input) {\n    organization { " + organizationFields + " }\n  }\n}"

	var data map[string]json.RawMessage
	if err := r.client.Mutate(ctx, doc, map[string]any{"input": in}, &data); err != nil {
		diags.AddError("Unable to update Linear workspace settings", err.Error())
		return
	}
	var payload map[string]json.RawMessage
	if err := decodeField(data, "organizationUpdate", &payload); err != nil {
		diags.AddError("Unable to update Linear workspace settings", err.Error())
		return
	}
	var raw json.RawMessage
	if err := decodeField(payload, "organization", &raw); err != nil {
		diags.AddError("Unable to update Linear workspace settings", err.Error())
		return
	}
	if err := plan.decode(ctx, raw); err != nil {
		diags.AddError("Unable to read Linear workspace settings after update", err.Error())
		return
	}
	diags.Append(state.Set(ctx, plan)...)
}

func (r *workspaceSettingsResource) readOrganization(ctx context.Context) (json.RawMessage, error) {
	doc := "query organization {\n  organization { " + organizationFields + " }\n}"
	var data map[string]json.RawMessage
	if err := r.client.Query(ctx, doc, nil, &data); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := decodeField(data, "organization", &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
