resource "linear_git_automation_target_branch" "main" {
  team_id        = linear_team.engineering.id
  branch_pattern = "main"
}

resource "linear_git_automation_target_branch" "release" {
  team_id        = linear_team.engineering.id
  branch_pattern = "^release/.+$"
  is_regex       = true
}
