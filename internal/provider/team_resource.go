package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Team is the widest entity Linear exposes, and its create input is markedly
// narrower than its update input — a dozen attributes (the AI toggles, the join
// settings, defaultIssueStateId, securitySettings, …) can only be set by an
// update. The model therefore implements createThenUpdate so the first apply
// still lands the whole configuration.
//
// Cycles are configured here, not as their own resource: Linear generates cycles
// itself once cyclesEnabled is set, so a linear_cycle resource would fight the
// generator. Duration, cooldown and start day are team settings and live here.
var teamEntity = entity{
	name: "team",
	fields: `id name key description icon color private timezone
		cyclesEnabled cycleStartDay cycleDuration cycleCooldownTime cycleLockToActive
		cycleIssueAutoAssignStarted cycleIssueAutoAssignCompleted upcomingCycleCount
		triageEnabled requirePriorityToLeaveTriage
		autoArchivePeriod autoClosePeriod autoCloseStateId autoCloseParentIssues autoCloseChildIssues
		defaultIssueEstimate issueEstimationType issueEstimationAllowZero issueEstimationExtended
		groupIssueHistory setIssueSortOrderOnStateChange initiativesEnabled
		inheritIssueEstimation inheritWorkflowStatuses inheritSlackAutoCreateProjectChannel
		slackAutoCreateProjectChannel aiThreadSummariesEnabled aiDiscussionSummariesEnabled
		allMembersCanJoin joinByDefault securitySettings
		parent { id } defaultIssueState { id } defaultProjectTemplate { id }
		defaultTemplateForMembers { id } defaultTemplateForNonMembers { id }`,
}

// NewTeamResource returns a new linear_team resource.
func NewTeamResource() resource.Resource {
	return &standardResource{
		entity:   teamEntity,
		typeName: "team",
		kind:     "team",
		schema:   teamSchema,
		newModel: func() crudModel { return &teamModel{} },
	}
}

type teamAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Key         string  `json:"key"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	Color       *string `json:"color"`
	Private     bool    `json:"private"`
	Timezone    string  `json:"timezone"`

	CyclesEnabled                 bool    `json:"cyclesEnabled"`
	CycleStartDay                 float64 `json:"cycleStartDay"`
	CycleDuration                 float64 `json:"cycleDuration"`
	CycleCooldownTime             float64 `json:"cycleCooldownTime"`
	CycleLockToActive             bool    `json:"cycleLockToActive"`
	CycleIssueAutoAssignStarted   bool    `json:"cycleIssueAutoAssignStarted"`
	CycleIssueAutoAssignCompleted bool    `json:"cycleIssueAutoAssignCompleted"`
	UpcomingCycleCount            float64 `json:"upcomingCycleCount"`

	TriageEnabled                bool `json:"triageEnabled"`
	RequirePriorityToLeaveTriage bool `json:"requirePriorityToLeaveTriage"`

	AutoArchivePeriod     float64  `json:"autoArchivePeriod"`
	AutoClosePeriod       *float64 `json:"autoClosePeriod"`
	AutoCloseStateID      *string  `json:"autoCloseStateId"`
	AutoCloseParentIssues *bool    `json:"autoCloseParentIssues"`
	AutoCloseChildIssues  *bool    `json:"autoCloseChildIssues"`

	DefaultIssueEstimate     float64 `json:"defaultIssueEstimate"`
	IssueEstimationType      string  `json:"issueEstimationType"`
	IssueEstimationAllowZero bool    `json:"issueEstimationAllowZero"`
	IssueEstimationExtended  bool    `json:"issueEstimationExtended"`

	GroupIssueHistory              bool   `json:"groupIssueHistory"`
	SetIssueSortOrderOnStateChange string `json:"setIssueSortOrderOnStateChange"`
	InitiativesEnabled             bool   `json:"initiativesEnabled"`

	InheritIssueEstimation               bool  `json:"inheritIssueEstimation"`
	InheritWorkflowStatuses              bool  `json:"inheritWorkflowStatuses"`
	InheritSlackAutoCreateProjectChannel bool  `json:"inheritSlackAutoCreateProjectChannel"`
	SlackAutoCreateProjectChannel        *bool `json:"slackAutoCreateProjectChannel"`

	AIThreadSummariesEnabled     bool `json:"aiThreadSummariesEnabled"`
	AIDiscussionSummariesEnabled bool `json:"aiDiscussionSummariesEnabled"`

	AllMembersCanJoin *bool `json:"allMembersCanJoin"`
	JoinByDefault     *bool `json:"joinByDefault"`

	SecuritySettings json.RawMessage `json:"securitySettings"`

	Parent                       *ref `json:"parent"`
	DefaultIssueState            *ref `json:"defaultIssueState"`
	DefaultProjectTemplate       *ref `json:"defaultProjectTemplate"`
	DefaultTemplateForMembers    *ref `json:"defaultTemplateForMembers"`
	DefaultTemplateForNonMembers *ref `json:"defaultTemplateForNonMembers"`
}

type teamModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Key         types.String `tfsdk:"key"`
	Description types.String `tfsdk:"description"`
	Icon        types.String `tfsdk:"icon"`
	Color       types.String `tfsdk:"color"`
	Private     types.Bool   `tfsdk:"private"`
	Timezone    types.String `tfsdk:"timezone"`
	ParentID    types.String `tfsdk:"parent_id"`

	CyclesEnabled                 types.Bool    `tfsdk:"cycles_enabled"`
	CycleStartDay                 types.Float64 `tfsdk:"cycle_start_day"`
	CycleDuration                 types.Int64   `tfsdk:"cycle_duration"`
	CycleCooldownTime             types.Int64   `tfsdk:"cycle_cooldown_time"`
	CycleLockToActive             types.Bool    `tfsdk:"cycle_lock_to_active"`
	CycleIssueAutoAssignStarted   types.Bool    `tfsdk:"cycle_issue_auto_assign_started"`
	CycleIssueAutoAssignCompleted types.Bool    `tfsdk:"cycle_issue_auto_assign_completed"`
	UpcomingCycleCount            types.Float64 `tfsdk:"upcoming_cycle_count"`

	TriageEnabled                types.Bool `tfsdk:"triage_enabled"`
	RequirePriorityToLeaveTriage types.Bool `tfsdk:"require_priority_to_leave_triage"`

	AutoArchivePeriod     types.Float64 `tfsdk:"auto_archive_period"`
	AutoClosePeriod       types.Float64 `tfsdk:"auto_close_period"`
	AutoCloseStateID      types.String  `tfsdk:"auto_close_state_id"`
	AutoCloseParentIssues types.Bool    `tfsdk:"auto_close_parent_issues"`
	AutoCloseChildIssues  types.Bool    `tfsdk:"auto_close_child_issues"`

	DefaultIssueEstimate     types.Float64 `tfsdk:"default_issue_estimate"`
	IssueEstimationType      types.String  `tfsdk:"issue_estimation_type"`
	IssueEstimationAllowZero types.Bool    `tfsdk:"issue_estimation_allow_zero"`
	IssueEstimationExtended  types.Bool    `tfsdk:"issue_estimation_extended"`

	GroupIssueHistory              types.Bool   `tfsdk:"group_issue_history"`
	SetIssueSortOrderOnStateChange types.String `tfsdk:"set_issue_sort_order_on_state_change"`
	InitiativesEnabled             types.Bool   `tfsdk:"initiatives_enabled"`

	InheritIssueEstimation               types.Bool `tfsdk:"inherit_issue_estimation"`
	InheritWorkflowStatuses              types.Bool `tfsdk:"inherit_workflow_statuses"`
	InheritSlackAutoCreateProjectChannel types.Bool `tfsdk:"inherit_slack_auto_create_project_channel"`
	SlackAutoCreateProjectChannel        types.Bool `tfsdk:"slack_auto_create_project_channel"`

	AIThreadSummariesEnabled     types.Bool `tfsdk:"ai_thread_summaries_enabled"`
	AIDiscussionSummariesEnabled types.Bool `tfsdk:"ai_discussion_summaries_enabled"`

	AllMembersCanJoin types.Bool `tfsdk:"all_members_can_join"`
	JoinByDefault     types.Bool `tfsdk:"join_by_default"`

	DefaultIssueStateID            types.String `tfsdk:"default_issue_state_id"`
	DefaultProjectTemplateID       types.String `tfsdk:"default_project_template_id"`
	DefaultTemplateForMembersID    types.String `tfsdk:"default_template_for_members_id"`
	DefaultTemplateForNonMembersID types.String `tfsdk:"default_template_for_non_members_id"`

	SecuritySettingsJSON jsontypes.Normalized `tfsdk:"security_settings_json"`

	// Write-only: Linear accepts these on the input but does not expose them on
	// the Team type, so there is nothing to read back.
	IssueSharingEnabled             types.Bool   `tfsdk:"issue_sharing_enabled"`
	ProductIntelligenceScope        types.String `tfsdk:"product_intelligence_scope"`
	InheritProductIntelligenceScope types.Bool   `tfsdk:"inherit_product_intelligence_scope"`
}

func (m *teamModel) id() string { return m.ID.ValueString() }

func (m *teamModel) decode(_ context.Context, raw json.RawMessage) error {
	var a teamAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Key = types.StringValue(a.Key)
	m.Description = types.StringPointerValue(a.Description)
	m.Icon = types.StringPointerValue(a.Icon)
	m.Color = types.StringPointerValue(a.Color)
	m.Private = types.BoolValue(a.Private)
	m.Timezone = types.StringValue(a.Timezone)
	m.ParentID = keepCleared(m.ParentID, refID(a.Parent))

	m.CyclesEnabled = types.BoolValue(a.CyclesEnabled)
	m.CycleStartDay = types.Float64Value(a.CycleStartDay)
	m.CycleDuration = types.Int64Value(int64(a.CycleDuration))
	m.CycleCooldownTime = types.Int64Value(int64(a.CycleCooldownTime))
	m.CycleLockToActive = types.BoolValue(a.CycleLockToActive)
	m.CycleIssueAutoAssignStarted = types.BoolValue(a.CycleIssueAutoAssignStarted)
	m.CycleIssueAutoAssignCompleted = types.BoolValue(a.CycleIssueAutoAssignCompleted)
	m.UpcomingCycleCount = types.Float64Value(a.UpcomingCycleCount)

	m.TriageEnabled = types.BoolValue(a.TriageEnabled)
	m.RequirePriorityToLeaveTriage = types.BoolValue(a.RequirePriorityToLeaveTriage)

	m.AutoArchivePeriod = types.Float64Value(a.AutoArchivePeriod)
	m.AutoClosePeriod = types.Float64PointerValue(a.AutoClosePeriod)
	m.AutoCloseStateID = keepCleared(m.AutoCloseStateID, types.StringPointerValue(a.AutoCloseStateID))
	m.AutoCloseParentIssues = types.BoolPointerValue(a.AutoCloseParentIssues)
	m.AutoCloseChildIssues = types.BoolPointerValue(a.AutoCloseChildIssues)

	m.DefaultIssueEstimate = types.Float64Value(a.DefaultIssueEstimate)
	m.IssueEstimationType = types.StringValue(a.IssueEstimationType)
	m.IssueEstimationAllowZero = types.BoolValue(a.IssueEstimationAllowZero)
	m.IssueEstimationExtended = types.BoolValue(a.IssueEstimationExtended)

	m.GroupIssueHistory = types.BoolValue(a.GroupIssueHistory)
	m.SetIssueSortOrderOnStateChange = types.StringValue(a.SetIssueSortOrderOnStateChange)
	m.InitiativesEnabled = types.BoolValue(a.InitiativesEnabled)

	m.InheritIssueEstimation = types.BoolValue(a.InheritIssueEstimation)
	m.InheritWorkflowStatuses = types.BoolValue(a.InheritWorkflowStatuses)
	m.InheritSlackAutoCreateProjectChannel = types.BoolValue(a.InheritSlackAutoCreateProjectChannel)
	m.SlackAutoCreateProjectChannel = types.BoolPointerValue(a.SlackAutoCreateProjectChannel)

	m.AIThreadSummariesEnabled = types.BoolValue(a.AIThreadSummariesEnabled)
	m.AIDiscussionSummariesEnabled = types.BoolValue(a.AIDiscussionSummariesEnabled)

	m.AllMembersCanJoin = types.BoolPointerValue(a.AllMembersCanJoin)
	m.JoinByDefault = types.BoolPointerValue(a.JoinByDefault)

	m.DefaultIssueStateID = keepCleared(m.DefaultIssueStateID, refID(a.DefaultIssueState))
	m.DefaultProjectTemplateID = keepCleared(m.DefaultProjectTemplateID, refID(a.DefaultProjectTemplate))
	m.DefaultTemplateForMembersID = keepCleared(m.DefaultTemplateForMembersID, refID(a.DefaultTemplateForMembers))
	m.DefaultTemplateForNonMembersID = keepCleared(
		m.DefaultTemplateForNonMembersID, refID(a.DefaultTemplateForNonMembers))

	m.SecuritySettingsJSON = jsonAttr(a.SecuritySettings)
	return nil
}

// createOnlyOmitted lists what TeamCreateInput cannot carry. Used by
// needsUpdateAfterCreate so the first apply still lands them.
func (m *teamModel) needsUpdateAfterCreate() bool {
	set := func(vs ...interface{ IsNull() bool }) bool {
		for _, v := range vs {
			if !v.IsNull() {
				return true
			}
		}
		return false
	}
	return set(
		m.AIThreadSummariesEnabled, m.AIDiscussionSummariesEnabled,
		m.AllMembersCanJoin, m.JoinByDefault,
		m.AutoCloseParentIssues, m.AutoCloseChildIssues,
		m.DefaultIssueStateID, m.SecuritySettingsJSON,
	)
}

func (m *teamModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"name": m.Name.ValueString()}

	// Every readable attribute passes clear=false: they are all Optional +
	// Computed, so an absent value means "keep what is live" rather than "clear
	// it". Sending null for what the configuration does not mention would erase
	// settings a human set in the UI — an icon, a default issue state — on the
	// first apply after import.
	putString(in, "key", m.Key, false)
	putString(in, "description", m.Description, false)
	putString(in, "icon", m.Icon, false)
	putString(in, "color", m.Color, false)
	putBool(in, "private", m.Private, false)
	putString(in, "timezone", m.Timezone, false)
	// The reference attributes go through putRef, which reads `""` as an explicit
	// clear — the one intent Optional + Computed otherwise leaves unsayable.
	putRef(in, "parentId", m.ParentID)

	putBool(in, "cyclesEnabled", m.CyclesEnabled, false)
	putFloat(in, "cycleStartDay", m.CycleStartDay, false)
	putInt(in, "cycleDuration", m.CycleDuration, false)
	putInt(in, "cycleCooldownTime", m.CycleCooldownTime, false)
	putBool(in, "cycleLockToActive", m.CycleLockToActive, false)
	putBool(in, "cycleIssueAutoAssignStarted", m.CycleIssueAutoAssignStarted, false)
	putBool(in, "cycleIssueAutoAssignCompleted", m.CycleIssueAutoAssignCompleted, false)
	putFloat(in, "upcomingCycleCount", m.UpcomingCycleCount, false)

	putBool(in, "triageEnabled", m.TriageEnabled, false)
	putBool(in, "requirePriorityToLeaveTriage", m.RequirePriorityToLeaveTriage, false)

	putFloat(in, "autoArchivePeriod", m.AutoArchivePeriod, false)
	putFloat(in, "autoClosePeriod", m.AutoClosePeriod, false)
	putRef(in, "autoCloseStateId", m.AutoCloseStateID)

	putFloat(in, "defaultIssueEstimate", m.DefaultIssueEstimate, false)
	putString(in, "issueEstimationType", m.IssueEstimationType, false)
	putBool(in, "issueEstimationAllowZero", m.IssueEstimationAllowZero, false)
	putBool(in, "issueEstimationExtended", m.IssueEstimationExtended, false)

	putBool(in, "groupIssueHistory", m.GroupIssueHistory, false)
	putString(in, "setIssueSortOrderOnStateChange", m.SetIssueSortOrderOnStateChange, false)
	putBool(in, "initiativesEnabled", m.InitiativesEnabled, false)

	putBool(in, "inheritIssueEstimation", m.InheritIssueEstimation, false)
	putBool(in, "inheritWorkflowStatuses", m.InheritWorkflowStatuses, false)
	putBool(in, "inheritSlackAutoCreateProjectChannel", m.InheritSlackAutoCreateProjectChannel, false)
	putBool(in, "slackAutoCreateProjectChannel", m.SlackAutoCreateProjectChannel, false)

	putRef(in, "defaultProjectTemplateId", m.DefaultProjectTemplateID)
	putRef(in, "defaultTemplateForMembersId", m.DefaultTemplateForMembersID)
	putRef(in, "defaultTemplateForNonMembersId", m.DefaultTemplateForNonMembersID)

	// Write-only: on both team inputs, on neither the Team type nor the
	// selection set above.
	putBool(in, "issueSharingEnabled", m.IssueSharingEnabled, false)
	putString(in, "productIntelligenceScope", m.ProductIntelligenceScope, false)
	putBool(in, "inheritProductIntelligenceScope", m.InheritProductIntelligenceScope, false)

	// Only on TeamUpdateInput — sending them on create is a validation error.
	if forUpdate {
		putBool(in, "aiThreadSummariesEnabled", m.AIThreadSummariesEnabled, false)
		putBool(in, "aiDiscussionSummariesEnabled", m.AIDiscussionSummariesEnabled, false)
		putBool(in, "allMembersCanJoin", m.AllMembersCanJoin, false)
		putBool(in, "joinByDefault", m.JoinByDefault, false)
		putBool(in, "autoCloseParentIssues", m.AutoCloseParentIssues, false)
		putBool(in, "autoCloseChildIssues", m.AutoCloseChildIssues, false)
		putRef(in, "defaultIssueStateId", m.DefaultIssueStateID)
		_ = putJSON(in, "securitySettings", m.SecuritySettingsJSON, false)
	}
	return in
}

func teamSchema() schema.Schema {
	// Every attribute Linear reports back is Optional + Computed, so an attribute
	// the configuration leaves out keeps its live value rather than being reset.
	// The trade-off is deliberate and the one linear_workspace_settings already
	// accepted: removing an attribute from the configuration no longer clears it,
	// so unsetting one means setting it explicitly. On a resource whose fields are
	// workspace state a human also edits in the UI, silently erasing what nobody
	// wrote down is the worse failure by far.
	//
	// The exceptions are the write-only attributes at the bottom, which Linear
	// accepts but never returns — Computed there would fail every apply.
	optBool := func(desc string) schema.Attribute {
		return schema.BoolAttribute{
			MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: keepBool(),
		}
	}
	optString := func(desc string) schema.Attribute {
		return schema.StringAttribute{
			MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: keepString(),
		}
	}
	optFloat := func(desc string) schema.Attribute {
		return schema.Float64Attribute{
			MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: keepFloat(),
		}
	}
	optInt := func(desc string) schema.Attribute {
		return schema.Int64Attribute{
			MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: keepInt(),
		}
	}

	return schema.Schema{
		MarkdownDescription: "Manages a Linear team — the container for issues, cycles, workflow states and " +
			"templates.\n\n" +
			"Cycle configuration lives here rather than in a resource of its own: Linear generates cycles itself " +
			"once `cycles_enabled` is set, so a cycle resource would fight the generator.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the team.",
				Required:            true,
			},
			// No keepString() here, deliberately: the key is derived from the
			// name, so on a rename it really is not knowable until Linear has
			// answered. See plan.go and derivedFromAnotherAttribute.
			"key": schema.StringAttribute{
				MarkdownDescription: "Team key — the prefix of every issue identifier, e.g. `ENG` in `ENG-42`. " +
					"Linear derives one from the name when unset.",
				Optional: true,
				Computed: true,
			},
			"description": optString("Description of the team."),
			"icon":        optString("Icon of the team."),
			"color":       optString("Colour of the team as a hex string."),
			"private":     optBool("Whether the team is private — visible only to its members."),
			"timezone":    optString("Timezone the team's cycles and SLAs are computed in, e.g. `Europe/Berlin`."),
			"parent_id":   optString("UUID of the parent team this team nests under." + clearWithEmptyString),

			"cycles_enabled":                    optBool("Whether Linear generates cycles for the team."),
			"cycle_start_day":                   optFloat("Weekday cycles start on, `0` being Sunday."),
			"cycle_duration":                    optInt("Length of a cycle in weeks."),
			"cycle_cooldown_time":               optInt("Cooldown between cycles in weeks."),
			"cycle_lock_to_active":              optBool("Whether issues can only be assigned to the active cycle."),
			"cycle_issue_auto_assign_started":   optBool("Whether starting an issue assigns it to the active cycle."),
			"cycle_issue_auto_assign_completed": optBool("Whether completing an issue assigns it to the active cycle."),
			"upcoming_cycle_count":              optFloat("How many upcoming cycles Linear keeps generated ahead."),

			"triage_enabled":                   optBool("Whether the team has a triage inbox for incoming issues."),
			"require_priority_to_leave_triage": optBool("Whether an issue needs a priority before it can leave triage."),

			"auto_archive_period": optFloat("Months of inactivity after which a closed issue is archived."),
			"auto_close_period": optFloat("Months of inactivity after which an open issue is closed. Linear " +
				"reports whether auto-closing is on at all, so removing the attribute keeps the live value rather " +
				"than disabling it — set it explicitly to change it."),
			"auto_close_state_id": optString(
				"UUID of the workflow state auto-closed issues move to." + clearWithEmptyString),
			"auto_close_parent_issues": optBool("Whether closing every sub-issue auto-closes the parent."),
			"auto_close_child_issues":  optBool("Whether closing a parent issue auto-closes its sub-issues."),

			"default_issue_estimate": optFloat("Estimate given to an issue that has none."),
			"issue_estimation_type": optString("Estimation scale — one of `notUsed`, `exponential`, `fibonacci`, " +
				"`linear`, `tShirt`."),
			"issue_estimation_allow_zero": optBool("Whether `0` is a valid estimate."),
			"issue_estimation_extended":   optBool("Whether the estimation scale is extended with larger values."),

			"group_issue_history": optBool("Whether the issue history groups consecutive changes by the same author."),
			"set_issue_sort_order_on_state_change": optString("Where an issue lands in the new state's list when " +
				"its state changes — `top`, `bottom` or `noChange`."),
			"initiatives_enabled": optBool("Whether the team's projects can belong to initiatives."),

			"inherit_issue_estimation":  optBool("Whether estimation settings are inherited from the parent team."),
			"inherit_workflow_statuses": optBool("Whether workflow states are inherited from the parent team."),
			"inherit_slack_auto_create_project_channel": optBool(
				"Whether the Slack project-channel setting is inherited from the parent team."),
			"slack_auto_create_project_channel": optBool(
				"Whether Linear creates a Slack channel for each new project of this team."),

			"ai_thread_summaries_enabled":     optBool("Whether Linear summarises long comment threads for this team."),
			"ai_discussion_summaries_enabled": optBool("Whether Linear summarises discussions for this team."),

			"all_members_can_join": optBool("Whether any workspace member can join the team without an invite."),
			"join_by_default":      optBool("Whether new workspace members join this team automatically."),

			"default_issue_state_id": optString(
				"UUID of the workflow state new issues start in." + clearWithEmptyString),
			"default_project_template_id": optString(
				"UUID of the `linear_template` new projects of this team start from." + clearWithEmptyString),
			"default_template_for_members_id": optString(
				"UUID of the `linear_template` used for issues created by team members." + clearWithEmptyString),
			"default_template_for_non_members_id": optString(
				"UUID of the `linear_template` used for issues created by people outside the team." +
					clearWithEmptyString),

			"security_settings_json": schema.StringAttribute{
				MarkdownDescription: "Team security settings as a JSON object — which role may manage what, e.g. " +
					"`jsonencode({ labelManagement = \"admin\", templateManagement = \"member\" })`. Keys: " +
					"`agentSkillsManagement`, `automationManagement`, `issueSharing`, `labelManagement`, " +
					"`memberManagement`, `teamManagement`, `templateManagement`. Compared semantically.",
				Optional:      true,
				Computed:      true,
				CustomType:    jsontypes.NormalizedType{},
				PlanModifiers: keepJSON(),
			},

			// Linear accepts these on the input but does not return them on the Team
			// type, so there is nothing to refresh them against. They are plain
			// Optional (not Computed): state keeps whatever the config declared, and
			// a change outside Terraform goes unnoticed.
			"issue_sharing_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether issues of this team can be shared with a public link. " +
					"**Write-only**: `TeamCreateInput` and `TeamUpdateInput` both take it, but the team does not " +
					"expose it, so drift in this attribute cannot be detected. Not to be confused with the " +
					"`issueSharing` key of `security_settings_json`, which is the role allowed to share rather than " +
					"whether sharing is on at all.",
				Optional: true,
			},
			"product_intelligence_scope": schema.StringAttribute{
				MarkdownDescription: "Scope product intelligence data is shared across — `none`, `team`, " +
					"`teamHierarchy` or `workspace`. **Write-only**: Linear accepts it on the team input but does " +
					"not expose it on the team, so drift in this attribute cannot be detected.",
				Optional: true,
			},
			"inherit_product_intelligence_scope": schema.BoolAttribute{
				MarkdownDescription: "Whether the product intelligence scope is inherited from the parent team. " +
					"**Write-only**, for the same reason as `product_intelligence_scope`.",
				Optional: true,
			},
		},
	}
}
