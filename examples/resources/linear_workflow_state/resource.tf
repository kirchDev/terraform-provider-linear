resource "linear_workflow_state" "in_review" {
  team_id  = linear_team.engineering.id
  name     = "In Review"
  color    = "#0f7488"
  type     = "started"
  position = 2
}

# `type` is a plain string, not an enum — which is what makes `duplicate`
# manageable here.
resource "linear_workflow_state" "duplicate" {
  team_id = linear_team.engineering.id
  name    = "Duplicate"
  color   = "#95a2b3"
  type    = "duplicate"
}
