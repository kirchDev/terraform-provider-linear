data "linear_label" "ai" {
  name = "ai"
}

# Narrow by team when a workspace label and a team label share a name:
data "linear_label" "flaky" {
  name    = "flaky"
  team_id = data.linear_team.engineering.id
}
