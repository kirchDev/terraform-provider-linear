data "linear_teams" "all" {}

output "team_keys" {
  value = [for t in data.linear_teams.all.teams : t.key]
}
