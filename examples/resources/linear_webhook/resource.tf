variable "linear_webhook_secret" {
  type      = string
  sensitive = true
}

resource "linear_webhook" "github_sync" {
  url     = "https://sync.example.com/linear"
  label   = "linear-github-sync"
  team_id = linear_team.engineering.id
  secret  = var.linear_webhook_secret

  resource_types = [
    "Issue",
    "Comment",
    "IssueLabel",
  ]
}
