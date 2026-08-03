resource "linear_release_pipeline" "web" {
  name          = "Web"
  type          = "continuous"
  is_production = true

  team_ids              = [linear_team.engineering.id]
  include_path_patterns = ["apps/web/**"]

  auto_generate_release_notes_on_completion = true
}
