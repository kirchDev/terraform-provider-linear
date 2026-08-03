resource "linear_agent_skill" "conventional_commits" {
  team_id = linear_team.engineering.id
  title   = "Conventional Commits"

  body = <<-EOT
    Write every commit message as a Conventional Commit.

    - `feat(scope): …` for a new capability
    - `fix(scope): …` for a bug fix
    - Breaking changes carry a `!` after the scope
  EOT
}
