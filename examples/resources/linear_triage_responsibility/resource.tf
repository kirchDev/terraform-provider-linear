resource "linear_triage_responsibility" "engineering" {
  team_id          = linear_team.engineering.id
  action           = "assign"
  time_schedule_id = linear_time_schedule.oncall.id
}

# Or a fixed list instead of a rota:
resource "linear_triage_responsibility" "design" {
  team_id = linear_team.design.id
  action  = "notify"

  manual_user_ids = [
    data.linear_user.alice.id,
    data.linear_user.bob.id,
  ]
}
