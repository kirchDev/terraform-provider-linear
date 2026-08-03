# The reason this provider exists. The filter goes through as JSON and is
# compared semantically, so Linear's server-side normalisation is not drift.
resource "linear_custom_view" "in_review" {
  name        = "In Review"
  description = "Everything waiting on a reviewer"
  team_id     = linear_team.engineering.id
  shared      = true
  color       = "#0f7488"

  filter_json = jsonencode({
    state  = { type = { eq = "started" } }
    labels = { some = { name = { eq = "ai" } } }
  })
}

# Set a different filter to make it a project, initiative or feed view — exactly
# one of the four may be set.
resource "linear_custom_view" "active_projects" {
  name   = "Active projects"
  shared = true

  project_filter_json = jsonencode({
    status = { type = { eq = "started" } }
  })
}
