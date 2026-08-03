resource "linear_workspace_label" "ai" {
  name        = "ai"
  color       = "#5e6ad2"
  description = "Touched by an AI agent"
}

# Labels nest: a group holds the labels underneath it.
resource "linear_workspace_label" "area" {
  name     = "area"
  color    = "#bec2c8"
  is_group = true
}

resource "linear_workspace_label" "area_infra" {
  name      = "infrastructure"
  color     = "#26b5ce"
  parent_id = linear_workspace_label.area.id
}
