resource "linear_view_preferences" "in_review" {
  custom_view_id = linear_custom_view.in_review.id
  view_type      = "customView"

  # Only the keys set here are read back, so drift is reported for the
  # preferences under management and no others.
  preferences_json = jsonencode({
    layout        = "board"
    issueGrouping = "assignee"
    viewOrdering  = "priority"
    showSubIssues = true
  })
}
