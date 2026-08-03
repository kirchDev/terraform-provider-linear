data "linear_workflow_states" "engineering" {
  team_id = data.linear_team.engineering.id
}

output "started_states" {
  value = [
    for s in data.linear_workflow_states.engineering.workflow_states :
    s.name if s.type == "started"
  ]
}
