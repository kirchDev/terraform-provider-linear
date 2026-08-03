# PLAN.md — building this provider

The work order for whoever picks this repo up. Read `CLAUDE.md` first for the
conventions; this file is what to build, in what order, and what is already done.

Delete this file once the provider ships v0.1.0 and the coverage table in
`README.md` matches reality.

---

## Status

**Phases 1–4 are implemented.** 25 resources, 12 data sources, the GraphQL
client, examples, generated `docs/`, and acceptance tests against an in-memory
GraphQL mock. `make build`, `make vet`, `make lint`, `make test` and
`make testacc` are green, and the binary loads in OpenTofu via `dev_overrides`.

What is **not** done, and has to happen before this file can be deleted:

1. **Nothing has run against a real workspace.** Every test drives a mock. The
   things a mock cannot tell you — whether Linear accepts each input field as
   modelled, what its defaults actually are, whether an Optional+Computed
   attribute really keeps its live value — are all still open. Point a
   `LINEAR_TOKEN` at a scratch workspace and walk the examples before release.
2. **The `dev` branch still does not exist** (caveat 4 below), so the first PR
   has nowhere to target and CI has never run.
3. **CodeQL's Go scan has never run** (caveat 3 below).
4. **Not published.** release-please → goreleaser → register `kirchdev/linear`
   with the OpenTofu registry.
5. **Phase 5** — adoption in `kirchDev/infrastructure` — untouched.

Two design questions the plan left open were resolved by reading the schema
rather than a live workspace, and both deserve a second look against the real
API:

- **`viewPreferences.preferences`** turned out to be `ViewPreferencesValues`, a
  typed object with 250+ fields, not the `JSONObject` the read side suggested.
  Selecting all of them would flood state with nulls, so the read selects
  exactly the keys the configuration sets. Consequence: drift is detected for
  the preferences under management and no others, and an import comes back with
  none until the first plan. See `view_preferences_resource.go`.
- **`CustomView.facet`** is not modelled, as the plan suspected it should not be.
- **`linear_view_preferences` is anchored to a custom view only.** Linear can
  attach preferences to a team, project or label too, but there is no top-level
  `viewPreferences(id:)` query, so those have no read path at all.

A handful of attributes are **write-only** — Linear accepts them on an input but
never returns them, so drift in them cannot be detected. They say so in their
description: `linear_team.product_intelligence_scope` and
`inherit_product_intelligence_scope`, `linear_workspace_settings.sla_enabled`,
`oauth_app_review` and `reduced_personal_information`,
`linear_custom_view.project_id` and `initiative_id`,
`linear_integrations_settings.custom_view_id`, and `linear_webhook.secret`
(that one deliberately, so it cannot leak through a refresh).

---

## Why this provider exists

`terraform-community-providers/linear` v0.3.7 is the newest published version and
has been unmoved for months. It covers **7 of ~25 managable Linear entities**, and
two of those seven are lacking inside the resource:

| Gap                                                                                                                          | Consequence                                                        |
| ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| No views ([PR #29](https://github.com/terraform-community-providers/terraform-provider-linear/pull/29), open since Nov 2023) | Every Linear view is clicked by hand                               |
| `workflow_state.type` modelled as an enum without `duplicate`                                                                | Every team's `Duplicate` state stays unmanaged                     |
| `team_workflow` bundles all git events, `merge` doesn't round-trip                                                           | Declaring `merge` means a permanent `+ merge` in every plan        |
| `workspace_settings` exposes 11 of ~50 `organizationUpdate` fields                                                           | Most workspace configuration isn't reachable from code             |
| ~18 entities missing entirely                                                                                                | Team memberships, triage, webhooks, project/release config by hand |

The consumer is `kirchDev/infrastructure` (`tofu/linear.tf`, `tofu/modules/linear/`,
SSOT in `tofu/data/linear/kirchdev.yml`). Its `CLAUDE.md` documents the same gaps
from the operator side — worth reading before designing schemas.

---

## What is already in place

Everything except the provider itself:

- **Meta layer** from `TitusKirch/scaffold` — pnpm, oxlint, oxfmt, husky, commitlint,
  release-please, issue/PR templates, CodeQL, Dependabot (with the `gomod` blocks
  and the security-update twins that keep labels working).
- **Go plumbing** — `go.mod` (`terraform-plugin-framework` v1.19, Go 1.25.8),
  `main.go`, `GNUmakefile`, `.golangci.yml`, `.prettierignore`.
- **Release plumbing** — `.goreleaser.yml`, `terraform-registry-manifest.json`,
  `KEYS` (the **same** GPG key the sibling providers use — already registered),
  `release-please.yml` with the goreleaser job wired to the **kirchDev-ci**
  Bitwarden mirrors.
- **CI** — `ci.yml` with a Go job (gofmt · vet · golangci-lint · build · test ·
  `TF_ACC` acceptance tests with OpenTofu installed) next to the lint job.
- **Scripts** — `scripts/gen-docs.sh` (tfplugindocs via dev_overrides),
  `scripts/fetch-schema.sh` (pulls the Linear SDL to `.linear-schema.graphql`).
- **`internal/provider/provider.go`** — provider schema (`token`, `endpoint`),
  `Configure` reading `LINEAR_TOKEN` / `LINEAR_ENDPOINT`, and **empty**
  `Resources()` / `DataSources()`.

### Known caveats in what's there

1. **`go.mod` / `go.sum` were copied from `terraform-provider-discord`.** The
   dependency set is right but unverified for this repo — run `make tidy` on a
   machine with Go ≥ 1.25 as the very first step. (The machine this was prepared
   on had Go 1.18, so `go build` was never executed here.)
2. **`terraform-plugin-framework-jsontypes` is not in `go.mod` yet.** It is
   required for the `*_json` attributes (see phase 3) — `go get` it in phase 2.
3. ~~**CodeQL scans `go` here**, unlike the sibling providers. Verify the first
   run is green; if `build-mode: none` misbehaves for Go, drop it back to
   `[actions, javascript-typescript]` rather than leaving a red required
   check.~~ **Resolved.** The first run failed: Go rejects `build-mode: none`
   outright. The build mode is now set per language — `autobuild` for Go, with
   `setup-go` reading `go.mod`, and `none` for the other two. Dropping `go`
   from the matrix was the alternative and was not taken: the reason for
   scanning it stands, and the failure was a one-line config error.
4. **No `dev` branch yet.** `.tituskirch-skills.json` sets `pr.base: dev` and
   `release.stages: [dev, main]`, and `ci.yml` / `codeql.yml` / `dependabot.yml`
   all reference `dev`. Create and push it before the first PR.
5. **The repo is not onboarded into `kirchDev/infrastructure` yet** — see the
   last section.

---

## Phase 1 — make it build

```bash
make tidy && make build && make vet && make test
pnpm install && pnpm check
```

Then push a `dev` branch and open the first PR so CI proves itself green before
any real code lands. Nothing below is worth starting while CI is unverified.

## Phase 2 — the GraphQL client (`internal/client/`)

**This is the one part the sibling providers cannot be copied from.** They speak
REST over per-resource paths; Linear is a single GraphQL endpoint.

Surface to build:

```go
func New(endpoint, token string) *Client
func (c *Client) Query(ctx context.Context, doc string, vars map[string]any, out any) error
func (c *Client) Mutate(ctx context.Context, doc string, vars map[string]any, out any) error
func NotFound(err error) bool
type APIError struct{ /* GraphQL errors[], typed */ }
```

Must-haves, each one a real trap:

- **`Authorization: <api key>`, no `Bearer` prefix.** Linear rejects the prefix.
- **Failures arrive as HTTP 200** with a populated `errors[]`. `NotFound()` keys
  off `extensions.type == "EntityNotFoundError"`, never off the status code. Get
  this wrong and a deleted entity never leaves the state — the next `plan` dies at
  refresh, and the only way out is `tofu state rm` against an encrypted remote
  state. The consumer repo has been there already, for labels.
- **Rate limit** 1500 req/h per key (`X-RateLimit-Requests-Remaining`). Honour
  `429` + `Retry-After`, retry transient `5xx` with backoff.
  `../terraform-provider-discord/internal/client/client.go:196-299` is the shape
  to copy, minus the REST specifics.
- **`client_test.go` against `httptest`** — query/mutation transport, `errors[]`
  parsing, `NotFound`, the rate-limit retry. Unit tests here are cheap and this is
  the layer every resource depends on.

Also in this phase: `go get github.com/hashicorp/terraform-plugin-framework-jsontypes`,
and wire the client into `Configure` (replace the `_ = endpoint` / `_ = token`
placeholder with `resp.ResourceData` / `resp.DataSourceData`).

## Phase 3 — resources

Conventions: one file per entity, one `*Attributes` struct, `New{Camel}Resource`,
TypeName `linear_{snake}`, registered in `provider.go`, `ImportState` on
everything. Look at `../terraform-provider-discord/internal/provider/role_resource.go`
for the CRUD shape.

Field definitions come from `.linear-schema.graphql` (`bash scripts/fetch-schema.sh`):

```bash
awk '/^input CustomViewCreateInput \{/,/^\}/' .linear-schema.graphql
```

### A. Parity — replaces the community provider

| Resource                              | Mutation                                     | Note                                                                      |
| ------------------------------------- | -------------------------------------------- | ------------------------------------------------------------------------- |
| `linear_workspace_settings`           | `organizationUpdate`                         | Singleton, manage-not-create: Create adopts, Delete is a no-op            |
| `linear_workspace_label`              | `issueLabelCreate` (no `teamId`)             | incl. `isGroup` / `parentId`                                              |
| `linear_team_label`                   | `issueLabelCreate` (with `teamId`)           | same shape                                                                |
| `linear_team`                         | `teamCreate` / `Update` / `Delete`           | 38 input fields                                                           |
| `linear_workflow_state`               | `workflowStateCreate` / `Update` / `Archive` | `type` is a **string**, seven values incl. `duplicate`                    |
| `linear_git_automation_state`         | `gitAutomationStateCreate` / …               | **one resource per event** — `draft`/`start`/`review`/`mergeable`/`merge` |
| `linear_git_automation_target_branch` | `gitAutomationTargetBranchCreate`            | own entity, so several branch patterns per team are possible              |
| `linear_template`                     | `templateCreate` / `Update` / `Delete`       | `templateData` → `template_json`                                          |

### A2. Attribute gaps — do not skip

Shipping a provider that covers `workspace_settings` more thinly than the one it
replaces would be a regression. Concretely:

- **`linear_workspace_settings`, 11 → ~50 attributes.** Flat fields:
  `gitBranchFormat`, `workingDays`, `fiscalYearStartMonth`, `slaEnabled`,
  `roadmapEnabled`, `releasesEnabled`, `defaultHomeView` (+ `defaultHomeViewTargetId`),
  `pullRequestIssueMode`, `allowedFileUploadContentTypes`, `hipaaComplianceEnabled`,
  the AI block (`aiAddonEnabled`, `agentAutomationEnabled`, `aiThreadSummariesEnabled`,
  `aiDiscussionSummariesEnabled`, `codeIntelligenceEnabled` + `codeIntelligenceRepository`,
  `codingAgentEnabled`, `restrictAgentInvocationToMembers`), the Slack project-channel
  block (`slackProjectChannelsEnabled`, `slackProjectChannelPrefix`,
  `slackAutoCreateProjectChannel`).
  As `*_json`: `securitySettings`, `authSettings`, `themeSettings`,
  `codingAgentSettings`, `linearAgentSettings`, `customersConfiguration`,
  `ipRestrictions`.
  **The deprecated flat fields (`allowMembersToInvite`, `restrictTeamCreationToAdmins`,
  `restrictLabelManagementToAdmins`) moved into `securitySettings` — expose one way
  in, not both.**
- **`linear_team`**, missing vs. the community provider: `defaultProjectTemplateId`,
  `defaultTemplateForMembersId`, `defaultTemplateForNonMembersId`,
  `inheritWorkflowStatuses`, `inheritIssueEstimation`, `issueSharingEnabled`,
  `setIssueSortOrderOnStateChange`, `productIntelligenceScope`.

> [!IMPORTANT]
> On the settings singleton every attribute is `Optional + Computed`. An
> undeclared field must keep its live value — with ~50 attributes on one resource,
> the alternative is an apply that silently resets a workspace.

### B. Views — the reason this repo exists

| Resource                  | Mutation                                 | Note                                                                |
| ------------------------- | ---------------------------------------- | ------------------------------------------------------------------- |
| `linear_custom_view`      | `customViewCreate` / `Update` / `Delete` | `team_id`, `project_id`, `initiative_id`, `shared`, `icon`, `color` |
| `linear_view_preferences` | `viewPreferencesCreate` / …              | layout / grouping / ordering, `type = organization`                 |

**Filter modelling — decided.** `IssueFilter` has 122 top-level fields, arbitrarily
nested, and the read side returns `filterData: JSONObject!` normalised by the
server. So: a `filter_json` string attribute typed as `jsontypes.Normalized`, which
compares semantically. A byte comparison drifts on every plan — this is exactly
where PR #29 stalled upstream.

Four parallel fields depending on `modelName`, exactly one of which may be set
(add a validator): `filter_json` (Issue), `project_filter_json`,
`initiative_filter_json`, `feed_item_filter_json`.

```hcl
resource "linear_custom_view" "eng_review" {
  name    = "In Review"
  team_id = linear_team.eng.id
  shared  = true

  filter_json = jsonencode({
    state  = { type = { eq = "started" } }
    labels = { some = { name = { eq = "ai" } } }
  })
}
```

Open question to resolve by reading the live API, not by guessing:
`viewPreferences.preferences` is an undocumented `JSONObject` — dump one from an
existing view before designing the attribute. Same for `CustomView.facet`, which
returns only an `id` and is probably a server-side derivation (the maintainer note
on PR #29 flags it); confirm before modelling it at all.

### C. Expansion — entities the community provider has no notion of

| Resource                       | Mutation                     | Why it belongs in TF                                                              |
| ------------------------------ | ---------------------------- | --------------------------------------------------------------------------------- |
| `linear_team_membership`       | `teamMembershipCreate`       | who is on which team, incl. `owner`                                               |
| `linear_triage_responsibility` | `triageResponsibilityCreate` | triage rotation per team                                                          |
| `linear_time_schedule`         | `timeScheduleCreate`         | on-call windows, the basis of that rotation                                       |
| `linear_project_status`        | `projectStatusCreate`        | workspace-wide project statuses — same class as workflow states                   |
| `linear_project_label`         | `projectLabelCreate`         | groups + members, same semantics as issue labels                                  |
| `linear_initiative_label`      | `initiativeLabelCreate`      | same                                                                              |
| `linear_webhook`               | `webhookCreate`              | currently hand-clicked; `linear-github-sync` depends on it. `secret` is sensitive |
| `linear_integrations_settings` | `integrationsSettingsCreate` | the Slack/Teams notification matrix — 15 booleans per team                        |
| `linear_release_pipeline`      | `releasePipelineCreate`      | `teamIds`, `includePathPatterns`, `isProduction`                                  |
| `linear_release_stage`         | `releaseStageCreate`         | stages per pipeline                                                               |
| `linear_emoji`                 | `emojiCreate`                | custom emoji (`name` + `url`)                                                     |
| `linear_agent_skill`           | `agentSkillCreate`           | agent skills per team                                                             |
| `linear_email_intake_address`  | `emailIntakeAddressCreate`   | intake address + auto-reply copy per team                                         |
| `linear_customer_status`       | `customerStatusCreate`       | **only if Linear Customers is in use** — confirm before building                  |
| `linear_customer_tier`         | `customerTierCreate`         | same                                                                              |

### D. Data sources

`linear_organization`, `linear_team` / `linear_teams`, `linear_user` / `linear_users`,
`linear_workflow_state` / `linear_workflow_states`, `linear_label` / `linear_labels`,
`linear_custom_view` / `linear_custom_views`, `linear_template`.

### E. Deliberately out of scope

Do not build these without a conversation first — each was considered and set aside:

- **Content**: `issue`, `project`, `initiative`, `document`, `comment`, `reaction`,
  `attachment`, `projectUpdate` / `initiativeUpdate`, `projectMilestone`,
  `release` / `releaseNote`, `customer`, `customerNeed`, every `*Relation`.
- **`cycle`** — Linear generates cycles itself when `cyclesEnabled`; a resource
  fights the generator. The _configuration_ (duration, cooldown, start day) sits on
  `linear_team`, where it belongs.
- **The 54 `integration*` mutations** — nearly all are OAuth connect flows
  (`integrationSlack`, `integrationGithubConnect`, …) that trade an authorization
  code for a token. Not idempotent, not declarative, not importable. What _is_
  managable is the layer after them: `integrations_settings` in C.
- **`favorite`** — 28 mutually exclusive foreign keys, and it is a personal sidebar
  ordering.
- **Events, not state**: `issueImport*`, `fileUpload`, `cycleShiftAll`,
  `organizationStartTrialForPlan`, `organizationDelete`, `logout*`, `passkey*`,
  `contactCreate`, `trackAnonymousEvent`.
- **Runtime**: `agentSession` / `agentActivity`, `notification*`, `pushSubscription`,
  user-scoped `viewPreferences`.
- **Borderline, decided against for v0.1.0**: `organization_domain` (verification
  runs over email, so an apply can't finish it), `organization_invite` (an invite is
  an event — once accepted the resource is orphaned), `user` (profile fields belong
  to the person; only a narrow `linear_user_role` would make sense), `roadmap`,
  `integration_template`, `oauth_application` (secret rotation in state),
  `notification_subscription`, `entity_external_link`.

## Phase 4 — docs, tests, release

- `examples/resources/linear_*/resource.tf` per resource, then `make docs`
  (tfplugindocs). `docs/` is generated — never hand-edit it.
- Acceptance tests `TF_ACC=1` against an in-memory **GraphQL** mock, structured
  like `../terraform-provider-discord/internal/provider/acctest_helpers_test.go`.
  Cover the structure-representative resources through apply → refresh → import →
  destroy, not every resource.
- **The view round-trip is the test that matters most**: create a
  `linear_custom_view` against the real workspace, then confirm `plan` is **empty**.
  That is the `filter_json` normalisation check; if it drifts, the semantic
  comparison isn't working.
- Release via release-please + goreleaser; then register `kirchdev/linear` with the
  OpenTofu registry (the address is free — verified 404 on
  `registry.opentofu.org/v1/providers/kirchDev/linear/versions`).

## Phase 5 — adopt it in `kirchDev/infrastructure`

Not this repo's work, but it is the point of the exercise, so the constraints are
recorded here:

1. `tofu/versions.tf`: source `terraform-community-providers/linear` →
   `kirchDev/linear`, pinned exactly. The local name stays `linear`, so nothing in
   `tofu/linear.tf` or the module has to be renamed.
2. **State migration is the hard part** — changing provider changes every
   resource's FQN, and `moved` does not work across providers.
   - Primary: `tofu state replace-provider` as a one-off, manually triggered CI job
     in the `tofu-state` concurrency group. The type names stay identical, which is
     what that command is for. Verify against a **copy** of the state first —
     attributes the new schema no longer has will fail to decode.
   - Fallback: `removed { … lifecycle { destroy = false } }` (apply 1) →
     `import` blocks (apply 2), with a `scripts/linear-generate-imports.sh` in the
     infra repo like the existing `*-generate-imports.sh`.
   - `linear_team_workflow` has **no 1:1 counterpart** (it becomes N
     `linear_git_automation_state`) — that part goes through removed + import either
     way.
3. Repo onboarding (create-then-import, as `dab1218` did for
   `terraform-provider-discord`): SSOT block in
   `tofu/data/github-orgs/kirchDev.yml` (`linear_team: OSS`, `has_issues: true`,
   `visibility: public`, the `base/community/ai/priority/release-please/dependabot/
github-actions/go/javascript` label packages, `required_code_scanning: true`),
   import blocks for the repo and the colliding scaffold default labels, then
   `scripts/check-labels.sh --fix` and `scripts/sync-security.sh`.
4. Afterwards: extend `tofu/data/linear/kirchdev.yml` with what was previously
   unmanageable (`Duplicate` states, the `merge` event, views) and delete the
   warning blocks at the top of that file.
