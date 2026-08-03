<div align="center">

# 🏗️ scaffold

**The kirchDev baseline — everything a new repo should ship with on day one, nothing more**

</div>

---

```bash
gh repo create my-new-repo --template TitusKirch/scaffold
```

That's it. Click **Use this template** (or use `gh`), edit a handful of placeholders, and the meta layer — lint, format, commit hooks, CI, CodeQL, Dependabot, release-please — is already wired up.

## ✨ What's in the box

- **🟢 Node + pnpm pinned** — `.nvmrc` (Node 24), `pnpm-workspace.yaml` (pnpm 11 with sane defaults), `package.json` with `packageManager`.
- **🧹 Lint & format via oxc** — `.oxlintrc.json`, `.oxfmtrc.json`, single `pnpm check` gate.
- **🪝 Commit hooks** — Husky + `lint-staged` + `commitlint` enforcing Conventional Commits.
- **🤖 Dependency PRs** — Dependabot (npm weekly, actions monthly) + `taze.config.js` for interactive upgrades.
- **🔁 release-please** — full workflow + config + manifest so the new repo can publish from its first commit.
- **🛡️ GitHub workflows** — `ci.yml` (lint + format check on PR), `codeql.yml` (push/PR + weekly).
- **📋 Issue / PR templates** — bug report, feature request, question (`.yml` forms) + PR checklist.
- **📄 Standard meta** — `LICENSE`, `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md`, `SECURITY.md`.
- **🤖 Agent-ready** — `CLAUDE.md` + `AGENTS.md` (kept in sync), `.tituskirch-skills.json`, baseline `.claude/settings.json` permissions, `pnpm skills:update` wiring.

The actual project code can be anything — PHP, Go, Rust, Vue, plain shell. `scaffold` only owns the meta layer that sits on top.

## 🚀 Setup

After clicking **Use this template**:

1. Clone your new repo.
2. Replace the placeholders listed in [Customising the template](#-customising-the-template).
3. Reset release-please as described in [Resetting release-please](#-resetting-release-please) (only if you want to start at `v0.0.0`).
4. `pnpm install` — Husky activates the hooks via the `prepare` script.
5. Add your project code and ship the first commit:

   ```bash
   git commit -m "chore: initial commit from scaffold"
   ```

## 🤖 AI & skills

Every repo from this template is agent-ready on day one:

- **`CLAUDE.md` + `AGENTS.md`** — one set of guidance for Claude Code and vendor-neutral agent tools (Codex, OpenCode, Cursor, Copilot). Kept **byte-identical** — edit one, edit the other.
- **`.claude/settings.json`** — baseline permissions: read-only git and the `pnpm` scripts are allowed; destructive git (`push`, `reset --hard`, `clean -f`, …) is denied.
- **`.tituskirch-skills.json`** — configures the [TitusKirch skills](https://github.com/TitusKirch/skills) (commit, PR, issue, release, docs) per repo.

Install the skill bundle, then keep project-scoped skills fresh:

```bash
pnpm dlx skills add TitusKirch/skills   # add the bundle — npx / yarn dlx / bunx work too
pnpm skills:update                       # refresh project-scoped skills
```

## 🧰 Customising the template

Every file below references `TitusKirch/scaffold`, the maintainer's name, or the maintainer's email. Search-and-replace these to your repo's identity before the first push.

| File                                  | Replace                                                                          |
| :------------------------------------ | :------------------------------------------------------------------------------- |
| `package.json`                        | `name`, `description`, `homepage`, `bugs.url`, `repository.url`, `author`        |
| `README.md`                           | Project title, tagline, hook snippet, every `TitusKirch/scaffold` link           |
| `LICENSE`                             | Copyright year + holder                                                          |
| `CODE_OF_CONDUCT.md`                  | Enforcement contact email                                                        |
| `CONTRIBUTING.md`                     | Every `TitusKirch/scaffold` link, the development setup section                  |
| `SECURITY.md`                         | Advisory URL, contact email, scope wording                                       |
| `.github/ISSUE_TEMPLATE/*.yml`        | Generic as shipped. `config.yml` links questions/ideas/possible-bugs to the Discord forum — private repos without a forum drop that block; optionally add stack-specific version fields to `bug_report.yml` |
| `.github/pull_request_template.md`    | Example commit message in the title hint                                         |
| `release-please-config.json`          | `packages["."]["package-name"]`                                                  |
| `CLAUDE.md` + `AGENTS.md`             | **Delete both** and regenerate with `/init` in Claude Code — scaffold-specific, keep byte-identical |

> [!TIP]
> A quick `grep -rn "TitusKirch/scaffold" .` catches every reference in one sweep.

> [!IMPORTANT]
> **Private repo?** Two defaults are public-only. Delete `.github/workflows/codeql.yml` (CodeQL needs GitHub Advanced Security — free only on public repos), and swap the MIT `LICENSE` + README footer for a proprietary notice with `package.json` `"license": "UNLICENSED"`.

## 🔁 Resetting release-please

`scaffold` ships with an initial manifest pinned at `0.0.0`. For most cases you can leave it alone — release-please will simply propose a first release PR after your first conventional commit on `main`. If you want a truly clean slate:

1. **Manifest** — make sure `.release-please-manifest.json` is `{ ".": "0.0.0" }` (the default).
2. **Changelog** — delete `CHANGELOG.md` if your fresh repo somehow inherited one.
3. **Config** — update `release-please-config.json` → `packages["."]["package-name"]` to your repo name.
4. **Workflow permissions** — in **Settings → Actions → General → Workflow permissions**, enable **Read and write permissions** so release-please can open its PR.
5. **Tags & releases (optional)** — if you copied the repo with history, drop old tags:

   ```bash
   git tag -l | xargs -r git tag -d
   ```

   …and clear any stale entries on the GitHub **Releases** tab.

6. **First commit** — push a Conventional Commit on `main` (`feat: …`, `fix: …`). release-please opens the initial release PR; merge it and your first tagged release ships.

## 💡 Why "scaffold" and not "template-\*"

Single word, brandable, language-neutral. Future stack-specific templates can sit next to it as `scaffold-laravel`, `scaffold-nuxt`, etc.

## 🤝 Contributing

PRs welcome. Conventional Commits required (enforced via commitlint). Husky runs the project's linters/formatters on `git commit`.

> [!TIP]
> Run `pnpm check:fix` before pushing — CI will catch what husky missed.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow.

## 🛣️ Versioning

[Semantic Versioning](https://semver.org/) via [release-please](https://github.com/googleapis/release-please) — see [CHANGELOG.md](CHANGELOG.md).

## 📄 License

[MIT](LICENSE) © [Titus Kirch](https://github.com/TitusKirch/) / [IT-Dienstleistungen Titus Kirch](https://kirch.dev)
