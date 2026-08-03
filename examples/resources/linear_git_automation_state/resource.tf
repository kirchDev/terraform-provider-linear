# One resource per git event. Bundling all five into one resource is what stops
# `merge` round-tripping elsewhere.
resource "linear_git_automation_state" "start" {
  team_id  = linear_team.engineering.id
  event    = "start"
  state_id = linear_workflow_state.in_progress.id
}

resource "linear_git_automation_state" "review" {
  team_id  = linear_team.engineering.id
  event    = "review"
  state_id = linear_workflow_state.in_review.id
}

resource "linear_git_automation_state" "merge" {
  team_id  = linear_team.engineering.id
  event    = "merge"
  state_id = linear_workflow_state.done.id
}

# A rule scoped to a branch pattern overrides the team default for that event.
resource "linear_git_automation_state" "merge_to_main" {
  team_id          = linear_team.engineering.id
  event            = "merge"
  state_id         = linear_workflow_state.released.id
  target_branch_id = linear_git_automation_target_branch.main.id
}
