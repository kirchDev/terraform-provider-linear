resource "linear_time_schedule" "oncall" {
  name = "Engineering on-call"

  entries = [
    {
      starts_at = "2026-01-05T09:00:00Z"
      ends_at   = "2026-01-12T09:00:00Z"
      user_id   = data.linear_user.alice.id
    },
    {
      starts_at = "2026-01-12T09:00:00Z"
      ends_at   = "2026-01-19T09:00:00Z"
      user_id   = data.linear_user.bob.id
    },
  ]
}
