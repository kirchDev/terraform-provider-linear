resource "linear_team" "engineering" {
  name = "Engineering"
  key  = "ENG"

  timezone       = "Europe/Berlin"
  triage_enabled = true

  cycles_enabled      = true
  cycle_duration      = 2
  cycle_cooldown_time = 0
  cycle_start_day     = 1

  issue_estimation_type = "fibonacci"
  auto_archive_period   = 3

  security_settings_json = jsonencode({
    labelManagement    = "admin"
    templateManagement = "member"
  })
}
