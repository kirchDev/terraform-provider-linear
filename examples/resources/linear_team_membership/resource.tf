data "linear_user" "alice" {
  email = "alice@example.com"
}

resource "linear_team_membership" "alice_engineering" {
  team_id = linear_team.engineering.id
  user_id = data.linear_user.alice.id
  owner   = true
}
