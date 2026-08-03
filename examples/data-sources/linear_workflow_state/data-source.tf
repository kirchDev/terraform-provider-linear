# State names are unique per team, so a lookup by name needs the team.
data "linear_workflow_state" "done" {
  team_id = data.linear_team.engineering.id
  name    = "Done"
}
