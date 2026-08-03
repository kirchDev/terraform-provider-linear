data "linear_custom_view" "in_review" {
  name = "In Review"
}

# Reading an existing view's filter is the quickest way to work out what to put
# in a linear_custom_view resource's filter_json.
output "filter" {
  value = data.linear_custom_view.in_review.filter_json
}
