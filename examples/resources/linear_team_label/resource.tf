resource "linear_team_label" "flaky" {
  team_id     = linear_team.engineering.id
  name        = "flaky"
  color       = "#f2c94c"
  description = "Fails intermittently in CI"
}
