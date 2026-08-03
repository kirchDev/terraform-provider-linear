resource "linear_email_intake_address" "support" {
  team_id     = linear_team.support.id
  type        = "team"
  template_id = linear_template.incident.id

  replies_enabled = true
  reopen_on_reply = true
  sender_name     = "kirchDev Support"

  issue_created_auto_reply_enabled = true
  issue_created_auto_reply         = "Thanks — we have your request and will come back to you."
}
