# CLAUDE.md

This file provides guidance to AI coding agents — Claude Code (claude.ai/code) and vendor-neutral tools such as Codex, OpenCode, Cursor, and Copilot — when working with code in this repository.

## Agent instruction files

`CLAUDE.md` and `AGENTS.md` are kept **byte-identical**. `CLAUDE.md` is what Claude Code reads; `AGENTS.md` is what vendor-neutral agent tools read — Codex, OpenCode, Cursor, Copilot, and whatever follows them. Two real files, deliberately not a symlink: not every tool resolves one.

**After editing either file, copy it over the other — don't repeat the edit by hand:**

```bash
cp CLAUDE.md AGENTS.md   # or the reverse, whichever you just edited
```

Retyping a change is exactly how the two drift; one reflowed line or reworded clause is enough. `diff CLAUDE.md AGENTS.md` must print nothing. If it ever does, treat it as a defect and fix it by letting one file win wholesale — never by merging them.

## What this repo is

A first-party **OpenTofu / Terraform provider for [Linear](https://linear.app)**, owned by kirchDev. It manages Linear _workspace configuration_ (teams, labels, workflow states, views, git automation, webhooks, workspace settings) as code so the Linear estate can live in the same IaC workflow as the rest of the kirchDev infrastructure.

- **Provider type (HCL):** `linear` → `provider "linear" {}`
- **OpenTofu registry address:** `kirchdev/linear`
- **Go module:** `github.com/kirchDev/terraform-provider-linear`
- **SDK:** `terraform-plugin-framework` (NOT the legacy SDKv2)

The repo name format `NAMESPACE/terraform-provider-NAME` is **mandatory** for the OpenTofu/Terraform registry — it's not a style choice.

**Scope is workspace configuration, not issue content** — there is no `linear_issue`, `linear_project` or `linear_document` resource. Content, `cycle` (Linear generates those itself), the 54 OAuth `integration*` mutations, `favorite`, and anything that is an event rather than state were all considered and deliberately left out. Don't add one without a conversation.

## Current state

> [!IMPORTANT]
> **25 resources and 12 data sources are implemented and released as v0.1.0; nothing has run against a real workspace.** Every test drives an in-memory mock, so whether Linear accepts each input field as modelled, and what its real defaults are, is still unverified.

Two layers coexist:

- **Node meta layer** (from the `scaffold` template): pnpm + oxlint + oxfmt + husky + commitlint + release-please + CI/CodeQL/Dependabot + issue/PR templates. Gates config/docs (JSON/YAML/MD).
- **Go provider**: `internal/client/` (the GraphQL client) and `internal/provider/` (resources, data sources, the shared CRUD machinery).

### How a resource file is put together

Linear's mutations are strikingly regular — `xCreate(input:)`, `x(id:)`, `xUpdate(id:, input:)`, `xDelete(id:)` — so that shape is implemented once and each resource contributes only what differs:

- **`graphql.go`** — `entity{name, fields, deleteVerb}` builds those four documents from a selection set. `connection()` pages through a collection query to the end.
- **`standard_resource.go`** — `standardResource` implements Create/Read/Update/Delete/ImportState against an `entity` plus a `crudModel` (`id()`, `input(ctx, forUpdate)`, `decode(ctx, raw)`). A model that also implements `createThenUpdate` gets a follow-up update after create, for entities whose create input is narrower than their update input (`linear_team` above all). A `readFn` replaces the `x(id:)` read where **Linear has no root query for the entity at all** — it takes the state model, not the bare id, because such an entity is only reachable through a parent whose id the model carries (`linear_git_automation_state` reads through `Team.gitAutomationStates`; on import, where the parent is not known yet, it searches the teams).
- **`helpers.go`** — the `put*` input builders. Their `clear` flag is the one subtlety: on **update** an attribute the user removed has to be sent as an explicit `null`, or removing it from the config is a silent no-op. Optional+Computed attributes pass `clear=false`, because for those an absent value legitimately means "keep what is live".
- **`standard_data_source.go`** — the same idea for data sources.

Resources that do not fit implement `resource.Resource` directly: `linear_workspace_settings` (a singleton with no id-based CRUD) and `linear_view_preferences` (no top-level read query — see its file).

**Attributes Linear accepts but never returns** are modelled as plain `Optional` (never `Computed`) and left untouched by `decode`, with **write-only** stated in their description. Marking them Computed instead would make Terraform reject the apply, because the value it planned is not the value that comes back.

## API shape (Linear GraphQL)

The sibling providers (`../terraform-provider-discord`, `../terraform-provider-laravelforge`) both speak REST. **Linear does not** — this is the one place where their patterns don't transfer.

- Single endpoint `https://api.linear.app/graphql`, everything is a `POST`. There are no per-resource paths, so the client exposes `Query`/`Mutate`, not `Get`/`List`/`Write`/`Delete`.
- **Auth is `Authorization: <api key>` — no `Bearer` prefix.** A personal API key is **workspace-scoped**, which is why multi-workspace setups need one aliased provider per workspace.
- **Errors come back as HTTP 200.** GraphQL reports failures in `errors[]`; the 404 equivalent is `extensions.type == "EntityNotFoundError"`. `NotFound()` must check that, not the status code — otherwise a `Read` never removes a deleted resource from state and the next `plan` dies at refresh.
- **Rate limit:** 1500 requests/hour per key, reported via `X-RateLimit-Requests-Remaining`. Honour `429` + `Retry-After` and retry transient `5xx` with backoff, so resources never see either.
- **Schema:** `bash scripts/fetch-schema.sh` pulls the full SDL (~1.2 MB, gitignored) to `.linear-schema.graphql`. That file is the field source of truth — which mutations exist, which inputs they take, what is nullable.

## Commands

Go (via `GNUmakefile`; needs **Go ≥ 1.25** — `terraform-plugin-framework v1.19` requires it):

| Command        | What it does                                                                  |
| :------------- | :---------------------------------------------------------------------------- |
| `make build`   | `go build -o terraform-provider-linear`                                       |
| `make tidy`    | `go mod tidy`                                                                 |
| `make fmt`     | `gofmt -s -w .`                                                               |
| `make vet`     | `go vet ./...`                                                                |
| `make lint`    | `golangci-lint run`                                                           |
| `make docs`    | render `docs/` from the schema (build + export + tfplugindocs)                |
| `make test`    | `go test ./...`                                                               |
| `make testacc` | `TF_ACC=1 go test ./...` — mock acceptance tests; needs a TF binary, no token |

Node meta layer: `pnpm install` (wires husky hooks), `pnpm check` / `pnpm check:fix`. CI (`.github/workflows/ci.yml`) runs a **Go job** (build·vet·gofmt·golangci-lint·test + `TF_ACC` mock acceptance tests, OpenTofu installed) and a **Lint job** (oxlint + oxfmt).

> [!NOTE]
> Generated files are excluded from oxfmt via `.prettierignore` (`docs/` from tfplugindocs, `CHANGELOG.md` from release-please). Don't reformat them — the next generation undoes it.

### Manual smoke test (loads the binary in OpenTofu)

```bash
make build
cat > /tmp/linear.tfrc <<EOF
provider_installation {
  dev_overrides { "kirchdev/linear" = "$(pwd)" }
  direct {}
}
EOF
TF_CLI_CONFIG_FILE=/tmp/linear.tfrc tofu -chdir=path/to/example validate
```

`validate` exercises the schema without calling the API; `plan`/`apply` would need a real `LINEAR_TOKEN`.

## Patterns & gotchas

- Follow the sibling providers' file conventions: one file per entity, one `*Attributes` struct, constructors `New{Camel}Resource` / `New{Camel}DataSource`, TypeName `linear_{snake}`, everything registered in `provider.go`.
- **Deeply-nested config goes through as raw JSON** in a `*_json` attribute — `filter_json`, `template_json`, `security_settings_json`. The entities that need this (`IssueFilter` alone has 122 top-level fields, arbitrarily nested) cannot be modelled as typed HCL without freezing against a schema that moves.
- **JSON attributes must compare semantically.** Linear normalises `filterData` server-side, so a byte comparison drifts on every plan. Use `jsontypes.Normalized` (`terraform-plugin-framework-jsontypes`), not `types.String`.
- **`workflow_state.type` is a plain string**, not an enum: `triage`, `backlog`, `unstarted`, `started`, `completed`, `canceled`, `duplicate`. The community provider's enum is exactly what makes `duplicate` unmanageable there.
- **Git automation is one entity per event.** `GitAutomationState` carries a single `event` (`draft` / `start` / `review` / `mergeable` / `merge`). Bundling all five into one resource is what breaks `merge` round-tripping in the community provider — don't repeat it.
- **`workspace_settings` is a singleton**: manage-not-create (Create adopts via `organizationUpdate`, Delete is a no-op). Every attribute stays `Optional + Computed`, so an undeclared field keeps its live value instead of being nulled.

## Release & publishing

- **release-please** (`.github/workflows/release-please.yml`, `release-type: go`) owns versioning + CHANGELOG + the tag + GitHub release. It runs on `main` with a **GitHub App token** minted from a Bitwarden-stored PEM — the **kirchDev-ci** mirror, not the `TitusKirch-ci` id the scaffold ships.
- When it cuts a release (`release_created == 'true'`), a second job in the **same workflow** runs **goreleaser**: builds the cross-platform archives, **GPG-signs** the checksums (key + passphrase from Bitwarden SM) and **appends** them to the release (`release.mode: append`).
- The registry consumes the per-platform zips + `SHA256SUMS` + detached `.sig` + `manifest.json` (protocol `6.0`, from `terraform-registry-manifest.json`).
- The GPG key in `KEYS` is the **same** key the sibling providers use — already registered, nothing to rotate.

## Conventions

- **Conventional Commits enforced** via commitlint on `git commit`. Don't `--no-verify` unless explicitly asked.
- **House style** for READMEs/meta files: centered hero block, prescribed section emojis (✨ 📦 🚀 🤝 🛣️ 📄), GitHub callouts (`> [!TIP]`), license footer `[MIT](LICENSE) © [Titus Kirch](https://github.com/TitusKirch/) / [IT-Dienstleistungen Titus Kirch](https://kirch.dev)`.
- Siblings **`../terraform-provider-laravelforge`** and **`../terraform-provider-discord`** are the reference implementations of these provider conventions — check them when unsure. Everything transfers except the client (REST there, GraphQL here).

## Don't relitigate

The decision to build our own provider rather than extend `terraform-community-providers/linear` v0.3.7 is settled. That provider covers 7 of ~25 managable entities, has no views ([PR #29](https://github.com/terraform-community-providers/terraform-provider-linear/pull/29) open since Nov 2023), models `workflow_state.type` as an enum that excludes `duplicate`, does not round-trip the `merge` git event, and exposes 11 of ~50 `organizationUpdate` fields.
