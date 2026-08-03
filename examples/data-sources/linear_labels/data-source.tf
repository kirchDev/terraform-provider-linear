data "linear_labels" "all" {}

data "linear_labels" "engineering" {
  team_id = data.linear_team.engineering.id
}
