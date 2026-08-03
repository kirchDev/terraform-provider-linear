package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

const userDataSourceFields = `id name displayName email active admin guest avatarUrl timezone url`

// NewUserDataSource returns a new linear_user data source.
func NewUserDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "user",
		kind:     "user",
		schema:   userDataSourceSchema,
		newModel: func() any { return &userDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readUser(ctx, c, model.(*userDataSourceModel))
		},
	}
}

// NewUsersDataSource returns a new linear_users data source.
func NewUsersDataSource() datasource.DataSource {
	return &standardDataSource{
		typeName: "users",
		kind:     "users",
		schema:   usersDataSourceSchema,
		newModel: func() any { return &usersDataSourceModel{} },
		read: func(ctx context.Context, c *client.Client, model any) error {
			return readUsers(ctx, c, model.(*usersDataSourceModel))
		},
	}
}

type userDataSourceAttributes struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	DisplayName string  `json:"displayName"`
	Email       string  `json:"email"`
	Active      bool    `json:"active"`
	Admin       bool    `json:"admin"`
	Guest       bool    `json:"guest"`
	AvatarURL   *string `json:"avatarUrl"`
	Timezone    *string `json:"timezone"`
	URL         string  `json:"url"`
}

type userDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Email       types.String `tfsdk:"email"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	Active      types.Bool   `tfsdk:"active"`
	Admin       types.Bool   `tfsdk:"admin"`
	Guest       types.Bool   `tfsdk:"guest"`
	AvatarURL   types.String `tfsdk:"avatar_url"`
	Timezone    types.String `tfsdk:"timezone"`
	URL         types.String `tfsdk:"url"`
}

func (m *userDataSourceModel) fill(a *userDataSourceAttributes) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.DisplayName = types.StringValue(a.DisplayName)
	m.Email = types.StringValue(a.Email)
	m.Active = types.BoolValue(a.Active)
	m.Admin = types.BoolValue(a.Admin)
	m.Guest = types.BoolValue(a.Guest)
	m.AvatarURL = types.StringPointerValue(a.AvatarURL)
	m.Timezone = types.StringPointerValue(a.Timezone)
	m.URL = types.StringValue(a.URL)
}

func readUser(ctx context.Context, c *client.Client, m *userDataSourceModel) error {
	if !m.ID.IsNull() && m.ID.ValueString() != "" {
		var raw json.RawMessage
		if err := (entity{name: "user", fields: userDataSourceFields}).read(ctx, c, m.ID.ValueString(), &raw); err != nil {
			return err
		}
		var a userDataSourceAttributes
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		m.fill(&a)
		return nil
	}

	if m.Email.IsNull() || m.Email.ValueString() == "" {
		return fmt.Errorf("set either id or email")
	}

	var users []userDataSourceAttributes
	err := connection(ctx, c, "users", "$filter: UserFilter", "filter: $filter", userDataSourceFields,
		map[string]any{"filter": map[string]any{"email": map[string]any{"eq": m.Email.ValueString()}}}, &users)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return fmt.Errorf("no user with email %q", m.Email.ValueString())
	}
	m.fill(&users[0])
	return nil
}

type usersDataSourceModel struct {
	IncludeDisabled types.Bool            `tfsdk:"include_disabled"`
	Users           []userDataSourceModel `tfsdk:"users"`
}

func readUsers(ctx context.Context, c *client.Client, m *usersDataSourceModel) error {
	// Linear returns deactivated members too; the default here is the set most
	// configurations mean by "the workspace's users".
	vars := map[string]any(nil)
	params, args := "", ""
	if m.IncludeDisabled.IsNull() || !m.IncludeDisabled.ValueBool() {
		params, args = "$filter: UserFilter", "filter: $filter"
		vars = map[string]any{"filter": map[string]any{"active": map[string]any{"eq": true}}}
	}

	var users []userDataSourceAttributes
	if err := connection(ctx, c, "users", params, args, userDataSourceFields, vars, &users); err != nil {
		return err
	}
	m.Users = make([]userDataSourceModel, 0, len(users))
	for i := range users {
		var one userDataSourceModel
		one.fill(&users[i])
		m.Users = append(m.Users, one)
	}
	return nil
}

func userDataSourceAttributeSchema(computedOnly bool) map[string]schema.Attribute {
	idAttr := schema.StringAttribute{MarkdownDescription: "UUID of the user.", Computed: true}
	emailAttr := schema.StringAttribute{MarkdownDescription: "Email address of the user.", Computed: true}
	if !computedOnly {
		idAttr.Optional = true
		idAttr.MarkdownDescription = "UUID of the user. Set either this or `email`."
		emailAttr.Optional = true
		emailAttr.MarkdownDescription = "Email address of the user. Set either this or `id`."
	}

	return map[string]schema.Attribute{
		"id":    idAttr,
		"email": emailAttr,
		"name": schema.StringAttribute{
			MarkdownDescription: "Full name of the user.",
			Computed:            true,
		},
		"display_name": schema.StringAttribute{
			MarkdownDescription: "Short name Linear shows the user under.",
			Computed:            true,
		},
		"active": schema.BoolAttribute{
			MarkdownDescription: "Whether the account is active rather than deactivated.",
			Computed:            true,
		},
		"admin": schema.BoolAttribute{
			MarkdownDescription: "Whether the user is a workspace admin.",
			Computed:            true,
		},
		"guest": schema.BoolAttribute{
			MarkdownDescription: "Whether the user is a guest, seeing only the teams they were invited to.",
			Computed:            true,
		},
		"avatar_url": schema.StringAttribute{
			MarkdownDescription: "URL of the user's avatar.",
			Computed:            true,
		},
		"timezone": schema.StringAttribute{
			MarkdownDescription: "Timezone the user works in.",
			Computed:            true,
		},
		"url": schema.StringAttribute{
			MarkdownDescription: "URL of the user's Linear profile.",
			Computed:            true,
		},
	}
}

func userDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Looks up a Linear user by UUID or email — the usual way to resolve the `user_id` a " +
			"`linear_team_membership` or `linear_triage_responsibility` needs.",
		Attributes: userDataSourceAttributeSchema(false),
	}
}

func usersDataSourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Lists the users of the Linear workspace.",
		Attributes: map[string]schema.Attribute{
			"include_disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether to include deactivated accounts. Defaults to `false`.",
				Optional:            true,
			},
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "The users, as Linear returns them.",
				Computed:            true,
				NestedObject:        schema.NestedAttributeObject{Attributes: userDataSourceAttributeSchema(true)},
			},
		},
	}
}
