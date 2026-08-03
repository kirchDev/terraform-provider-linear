data "linear_team" "engineering" {
  key = "ENG"
}

# Or by UUID:
data "linear_team" "by_id" {
  id = "a1b2c3d4-0000-0000-0000-000000000000"
}
