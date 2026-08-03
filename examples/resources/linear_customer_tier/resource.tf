resource "linear_customer_tier" "enterprise" {
  name         = "enterprise"
  display_name = "Enterprise"
  description  = "Contractual SLA, requests weighed highest"
  color        = "#eb5757"
  position     = 1
}

resource "linear_customer_tier" "free" {
  name         = "free"
  display_name = "Free"
  color        = "#bec2c8"
  position     = 3
}
