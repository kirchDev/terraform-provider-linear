resource "linear_project_status" "in_progress" {
  name     = "In Progress"
  color    = "#0f7488"
  type     = "started"
  position = 2
}

resource "linear_project_status" "maintained" {
  name        = "Maintained"
  description = "Shipped and cared for, with no end date"
  color       = "#4cb782"
  type        = "started"
  position    = 3
  indefinite  = true
}
