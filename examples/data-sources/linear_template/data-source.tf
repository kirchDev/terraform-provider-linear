# Linear exposes no template collection to search, so the lookup needs the UUID.
data "linear_template" "incident" {
  id = "a1b2c3d4-0000-0000-0000-000000000000"
}
