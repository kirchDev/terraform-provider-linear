resource "linear_template" "incident" {
  team_id = linear_team.engineering.id
  name    = "Incident"
  type    = "issue"
  color   = "#eb5757"

  template_json = jsonencode({
    title       = "Incident: "
    priority    = 1
    description = "## Impact\n\n## Timeline\n\n## Root cause\n"
  })
}
