<div align="center">

# 📐 terraform-provider-linear

**Manage your Linear workspace as code — teams, labels, workflow states, views, git automation and workspace settings, reconciled by OpenTofu**

[![Release](https://img.shields.io/github/v/release/kirchDev/terraform-provider-linear?style=flat-square&label=release&color=5E6AD2)](https://github.com/kirchDev/terraform-provider-linear/releases/latest)
[![OpenTofu Registry](https://img.shields.io/badge/opentofu-kirchdev%2Flinear-FFDA18?style=flat-square&logo=opentofu&logoColor=black)](https://search.opentofu.org/provider/kirchdev/linear/latest)
[![Terraform Registry](https://img.shields.io/badge/terraform-kirchdev%2Flinear-7b42bc?style=flat-square&logo=terraform&logoColor=white)](https://registry.terraform.io/providers/kirchDev/linear/latest)
[![Tests](https://img.shields.io/github/actions/workflow/status/kirchDev/terraform-provider-linear/ci.yml?branch=main&style=flat-square&label=tests)](https://github.com/kirchDev/terraform-provider-linear/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/kirchDev/terraform-provider-linear?style=flat-square&color=5E6AD2)](LICENSE)

</div>

---

```hcl
resource "linear_custom_view" "in_review" {
  name    = "In Review"
  team_id = linear_team.eng.id
  shared  = true

  filter_json = jsonencode({
    state = { type = { eq = "started" } }
  })
}
```

Teams, labels, workflow states, views and git automation declared in HCL and reconciled by OpenTofu — not clicked together in the Linear UI. **Scope is workspace configuration, not issue content.**

> [!IMPORTANT]
> **Pre-1.0 / beta.** Every resource and data source below is implemented and covered by acceptance tests against a GraphQL mock, but **nothing has been verified against a live workspace yet** — which fields Linear really accepts, and what its actual defaults are, is still unconfirmed. Pin an exact version and test before relying on it.

## 📦 Install & run

```hcl
terraform {
  required_providers {
    linear = {
      source  = "kirchdev/linear"
      version = "~> 0.1"
    }
  }
}

provider "linear" {
  token = var.linear_token # or set LINEAR_TOKEN
}

resource "linear_team" "eng" {
  name = "Engineering"
  key  = "ENG"
}
```

```bash
export LINEAR_TOKEN="lin_api_..."   # Linear → Settings → API → Personal API keys
tofu plan
```

> [!NOTE]
> A Linear API key is **workspace-scoped**. Managing several workspaces needs one aliased provider per workspace, each with its own key.

## ✨ Features

- **📐 Linear as code** — teams, labels, workflow states, views, git automation, webhooks and workspace settings in HCL.
- **🔭 Views included** — `linear_custom_view` with team, project and initiative scope. Filters are expressed as JSON and compared semantically, so a server-normalised filter doesn't read as drift.
- **🧩 Full workspace-settings coverage** — every field `organizationUpdate` accepts, not a subset.
- **🌿 Git automation per event** — `draft`, `start`, `review`, `mergeable` and `merge` each as their own resource, so all five round-trip on import.
- **🚀 OpenTofu & Terraform** — published as `kirchdev/linear` on both registries.
- **⚡ Modern stack** — `terraform-plugin-framework`; docs generated from the schema.

## 🗺️ Coverage

Scope is **workspace configuration**. Issues, projects, initiatives, documents and comments are content — they belong in Linear's UI and its API, not in a state file.

<details>
<summary>Full coverage</summary>

- **Workspace** — `linear_workspace_settings`, `linear_workspace_label`, `linear_project_status`, `linear_project_label`, `linear_initiative_label`, `linear_emoji`.
- **Teams** — `linear_team`, `linear_team_label`, `linear_team_membership`, `linear_workflow_state`, `linear_template`, `linear_triage_responsibility`, `linear_time_schedule`, `linear_email_intake_address`, `linear_agent_skill`.
- **Git automation** — `linear_git_automation_state`, `linear_git_automation_target_branch`.
- **Views** — `linear_custom_view`, `linear_view_preferences`.
- **Integrations** — `linear_webhook`, `linear_integrations_settings`.
- **Releases** — `linear_release_pipeline`, `linear_release_stage`.
- **Customers** — `linear_customer_status`, `linear_customer_tier`. Needs Linear Customers enabled for the workspace.
- **Data sources** — `linear_organization`, `linear_team(s)`, `linear_user(s)`, `linear_workflow_state(s)`, `linear_label(s)`, `linear_custom_view(s)`, `linear_template`.

</details>

## 📚 Documentation

Per-resource docs live under [`docs/`](docs/), generated from the schema with `make docs` (build + export schema + tfplugindocs).

## 🤝 Contributing

PRs welcome. Conventional Commits required (enforced via commitlint). Husky runs the linters/formatters on `git commit`.

> [!TIP]
> Run `make build && go vet ./...` before pushing — CI will catch what husky missed.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow.

## 🛣️ Versioning

[Semantic Versioning](https://semver.org/) via [release-please](https://github.com/googleapis/release-please) — see [CHANGELOG.md](CHANGELOG.md).

## 📄 License

[MIT](LICENSE) © [Titus Kirch](https://github.com/TitusKirch/) / [IT-Dienstleistungen Titus Kirch](https://kirch.dev)
