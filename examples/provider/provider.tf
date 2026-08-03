terraform {
  required_providers {
    linear = {
      source = "kirchdev/linear"
    }
  }
}

variable "linear_token" {
  type      = string
  sensitive = true
}

# A Linear API key is scoped to a single workspace, so managing several
# workspaces means one aliased provider per workspace.
provider "linear" {
  token = var.linear_token
}
