package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// One resource per git event, not one bundling all five. GitAutomationState
// carries a single `event`, and modelling the five as attributes of one resource
// is what stops `merge` round-tripping in the community provider: the read maps
// five entities onto one object and cannot tell an unset event from a deleted
// one, so a declared `merge` shows up as a permanent diff.
//
// The three mutations are the usual root-level ones, but the read is not: Linear
// has no gitAutomationState(id:) query, and no plural one either. A rule is only
// reachable through the team that owns it — Team.gitAutomationStates — so the
// read goes that way instead, and this entity contributes its selection set to
// the mutations only.
var gitAutomationStateEntity = entity{
	name:   "gitAutomationState",
	fields: `id event state { id } targetBranch { id } team { id }`,
}

// NewGitAutomationStateResource returns a new linear_git_automation_state resource.
func NewGitAutomationStateResource() resource.Resource {
	return &standardResource{
		entity:   gitAutomationStateEntity,
		typeName: "git_automation_state",
		kind:     "git automation state",
		schema:   gitAutomationStateSchema,
		newModel: func() crudModel { return &gitAutomationStateModel{} },
		readFn:   readGitAutomationState,
	}
}

// readGitAutomationState finds one rule among its team's, since there is no
// query that asks for it directly.
//
// A refresh knows the team — team_id is required and replace-forcing, so it is
// in state — and reads that one team. An import knows only the rule's UUID, so
// the team has to be found first: every team is asked for its rules until the
// one carrying this id turns up. That is a page of teams plus a query each, on
// import alone, and it is what keeps `terraform import` taking the plain UUID
// the resource's own id is.
func readGitAutomationState(ctx context.Context, c *client.Client, m crudModel) (json.RawMessage, error) {
	state, ok := m.(*gitAutomationStateModel)
	if !ok {
		return nil, fmt.Errorf("git automation state read got a %T", m)
	}

	var teamIDs []string
	if teamID := state.TeamID.ValueString(); teamID != "" {
		teamIDs = []string{teamID}
	} else {
		var teams []struct {
			ID string `json:"id"`
		}
		if err := connection(ctx, c, "teams", "", "", "id", nil, &teams); err != nil {
			return nil, err
		}
		for _, t := range teams {
			teamIDs = append(teamIDs, t.ID)
		}
	}

	for _, teamID := range teamIDs {
		rule, err := findGitAutomationState(ctx, c, teamID, state.ID.ValueString())
		if err != nil {
			return nil, err
		}
		if rule != nil {
			return rule, nil
		}
	}
	// Searched and not there: the same answer a missing entity gives everywhere
	// else, so Read drops it from state instead of failing the plan.
	return nil, notFoundError(gitAutomationStateEntity.name)
}

// findGitAutomationState returns the team's rule with this id, or nil if the
// team has no such rule — a team that is gone included, since it took its rules
// with it. Pagination is followed to the end: a rule on a page nobody read is
// indistinguishable from a deleted one, and Read would drop it from state.
func findGitAutomationState(ctx context.Context, c *client.Client, teamID, id string) (json.RawMessage, error) {
	doc := fmt.Sprintf("query gitAutomationStates($id: String!, $after: String) {\n"+
		"  team(id: $id) {\n"+
		"    gitAutomationStates(first: 250, after: $after) {\n"+
		"      nodes { %s }\n"+
		"      pageInfo { hasNextPage endCursor }\n"+
		"    }\n  }\n}", gitAutomationStateEntity.fields)

	var after *string
	for {
		var data map[string]json.RawMessage
		var team map[string]json.RawMessage

		err := c.Query(ctx, doc, map[string]any{"id": teamID, "after": after}, &data)
		if err == nil {
			err = decodeField(data, "team", &team)
		}
		switch {
		case client.NotFound(err):
			// The team is gone, and took its rules with it. That is this rule not
			// being there, not a failure — on import it is simply the wrong team.
			return nil, nil
		case err != nil:
			return nil, err
		}

		var rules struct {
			Nodes    []json.RawMessage `json:"nodes"`
			PageInfo struct {
				HasNextPage bool    `json:"hasNextPage"`
				EndCursor   *string `json:"endCursor"`
			} `json:"pageInfo"`
		}
		if err := decodeField(team, "gitAutomationStates", &rules); err != nil {
			return nil, err
		}

		for _, node := range rules.Nodes {
			var found struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(node, &found); err != nil {
				return nil, fmt.Errorf("decoding gitAutomationState: %w", err)
			}
			if found.ID == id {
				return node, nil
			}
		}

		if !rules.PageInfo.HasNextPage || rules.PageInfo.EndCursor == nil {
			return nil, nil
		}
		after = rules.PageInfo.EndCursor
	}
}

type gitAutomationStateAttributes struct {
	ID           string `json:"id"`
	Event        string `json:"event"`
	State        *ref   `json:"state"`
	TargetBranch *ref   `json:"targetBranch"`
	Team         *ref   `json:"team"`
}

type gitAutomationStateModel struct {
	ID             types.String `tfsdk:"id"`
	TeamID         types.String `tfsdk:"team_id"`
	Event          types.String `tfsdk:"event"`
	StateID        types.String `tfsdk:"state_id"`
	TargetBranchID types.String `tfsdk:"target_branch_id"`
}

func (m *gitAutomationStateModel) id() string { return m.ID.ValueString() }

func (m *gitAutomationStateModel) decode(_ context.Context, raw json.RawMessage) error {
	var a gitAutomationStateAttributes
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Event = types.StringValue(a.Event)
	m.StateID = refID(a.State)
	m.TargetBranchID = refID(a.TargetBranch)
	m.TeamID = refID(a.Team)
	return nil
}

func (m *gitAutomationStateModel) input(_ context.Context, forUpdate bool) map[string]any {
	in := map[string]any{"event": m.Event.ValueString()}
	// Both nulls carry meaning here, so they are sent explicitly on update:
	// stateId null means "take no action, overriding the default rule", and
	// targetBranchId null means "apply to every branch".
	putString(in, "stateId", m.StateID, forUpdate)
	putString(in, "targetBranchId", m.TargetBranchID, forUpdate)
	if !forUpdate {
		in["teamId"] = m.TeamID.ValueString()
	}
	return in
}

func gitAutomationStateSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages one git automation rule of a Linear team: when the given git event fires, " +
			"linked issues move to the given workflow state.\n\n" +
			"Each Linear `GitAutomationState` covers exactly one event, so five events means five resources. " +
			"Combined with `linear_git_automation_target_branch` this also expresses per-branch overrides — a rule " +
			"with `target_branch_id` set applies only to pull requests targeting that branch pattern and overrides " +
			"the team's default rule for the same event.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the automation rule.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the team the rule belongs to. Changing it replaces the rule — " +
					"`gitAutomationStateUpdate` has no `teamId`.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"event": schema.StringAttribute{
				MarkdownDescription: "Git event that triggers the rule — one of `draft` (draft PR opened), " +
					"`start` (branch created), `review` (PR ready for review), `mergeable` (PR approved and " +
					"mergeable) or `merge` (PR merged).",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("draft", "start", "review", "mergeable", "merge"),
				},
			},
			"state_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the workflow state linked issues move to. Leave unset to make the " +
					"rule take no action — which is how a team overrides an inherited default for this event.",
				Optional: true,
			},
			"target_branch_id": schema.StringAttribute{
				MarkdownDescription: "UUID of a `linear_git_automation_target_branch` this rule is scoped to. " +
					"Unset means the rule applies to pull requests against any branch.",
				Optional: true,
			},
		},
	}
}
