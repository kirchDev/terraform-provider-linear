package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// Ensure LinearProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*LinearProvider)(nil)

// defaultEndpoint is Linear's single GraphQL endpoint. Unlike a REST API there
// are no per-resource paths — every read and write is a POST to this URL.
const defaultEndpoint = "https://api.linear.app/graphql"

// LinearProvider is the provider implementation.
type LinearProvider struct {
	// version is set to the release version on build, or "dev" for local builds.
	version string
}

// LinearProviderModel maps provider schema data to a Go type.
type LinearProviderModel struct {
	Token    types.String `tfsdk:"token"`
	Endpoint types.String `tfsdk:"endpoint"`
}

// New returns a function that instantiates the provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &LinearProvider{version: version}
	}
}

func (p *LinearProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "linear"
	resp.Version = p.version
}

func (p *LinearProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage [Linear](https://linear.app) workspace configuration — teams, labels, workflow " +
			"states, views, git automation, webhooks and workspace settings — as code. An API key is scoped to a " +
			"single workspace, so managing several workspaces needs one aliased provider per workspace.",
		Attributes: map[string]schema.Attribute{
			"token": schema.StringAttribute{
				MarkdownDescription: "Linear API key. Sent verbatim as the `Authorization` header — Linear expects a " +
					"personal API key **without** a `Bearer` prefix. The key is workspace-scoped. " +
					"May also be set via the `LINEAR_TOKEN` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "URL of the Linear GraphQL API. Defaults to `" + defaultEndpoint + "`. " +
					"May also be set via `LINEAR_ENDPOINT` (mainly for testing).",
				Optional: true,
			},
		},
	}
}

func (p *LinearProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config LinearProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Unknown Linear API key",
			"The provider cannot create the Linear API client because the token is unknown. "+
				"Set the value statically in the configuration or via the LINEAR_TOKEN environment variable.",
		)
		return
	}

	// Env vars are the default; explicit config wins.
	token := os.Getenv("LINEAR_TOKEN")
	if !config.Token.IsNull() {
		token = config.Token.ValueString()
	}

	endpoint := os.Getenv("LINEAR_ENDPOINT")
	if !config.Endpoint.IsNull() {
		endpoint = config.Endpoint.ValueString()
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing Linear API key",
			"Set the provider `token` argument or the LINEAR_TOKEN environment variable.",
		)
		return
	}

	c := client.New(endpoint, token)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *LinearProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// Workspace configuration
		NewWorkspaceSettingsResource,

		// Labels
		NewWorkspaceLabelResource,
		NewTeamLabelResource,

		// Teams and their workflow
		NewTeamResource,
		NewWorkflowStateResource,
		NewGitAutomationStateResource,
		NewGitAutomationTargetBranchResource,
		NewTemplateResource,

		// Views
		NewCustomViewResource,
		NewViewPreferencesResource,

		// People and triage
		NewTeamMembershipResource,
		NewTriageResponsibilityResource,
		NewTimeScheduleResource,

		// Projects and initiatives
		NewProjectStatusResource,
		NewProjectLabelResource,
		NewInitiativeLabelResource,

		// Releases
		NewReleasePipelineResource,
		NewReleaseStageResource,

		// Customers — needs Linear Customers enabled for the workspace
		NewCustomerStatusResource,
		NewCustomerTierResource,

		// Integrations and intake
		NewWebhookResource,
		NewIntegrationsSettingsResource,
		NewEmailIntakeAddressResource,
		NewEmojiResource,
		NewAgentSkillResource,
	}
}

func (p *LinearProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOrganizationDataSource,

		NewTeamDataSource,
		NewTeamsDataSource,

		NewUserDataSource,
		NewUsersDataSource,

		NewWorkflowStateDataSource,
		NewWorkflowStatesDataSource,

		NewLabelDataSource,
		NewLabelsDataSource,

		NewCustomViewDataSource,
		NewCustomViewsDataSource,

		NewTemplateDataSource,
	}
}
