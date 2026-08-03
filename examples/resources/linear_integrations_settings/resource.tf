# Connecting Slack is an OAuth flow and not manageable declaratively; this is
# the layer above it — which events an existing connection posts.
resource "linear_integrations_settings" "engineering" {
  team_id = linear_team.engineering.id

  slack_issue_created             = true
  slack_issue_added_to_triage     = true
  slack_issue_status_changed_done = true
  slack_issue_sla_breached        = true
  slack_issue_new_comment         = false
}
