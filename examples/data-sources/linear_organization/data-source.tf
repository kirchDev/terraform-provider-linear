# Takes no arguments: the API key determines the workspace.
data "linear_organization" "this" {}

output "workspace" {
  value = data.linear_organization.this.url_key
}
