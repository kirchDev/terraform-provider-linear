data "linear_custom_views" "all" {}

output "shared_views" {
  value = [
    for v in data.linear_custom_views.all.custom_views :
    v.name if v.shared
  ]
}
