# Needs Linear Customers enabled for the workspace — see `customers_enabled` on
# linear_workspace_settings.
resource "linear_customer_status" "prospect" {
  name         = "prospect"
  display_name = "Prospect"
  description  = "In conversation, not yet signed"
  color        = "#bec2c8"
  position     = 1
}

resource "linear_customer_status" "active" {
  name         = "active"
  display_name = "Active"
  color        = "#4cb782"
  position     = 2
}
