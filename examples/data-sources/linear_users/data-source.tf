data "linear_users" "members" {}

# Deactivated accounts are excluded unless asked for:
data "linear_users" "everyone" {
  include_disabled = true
}
