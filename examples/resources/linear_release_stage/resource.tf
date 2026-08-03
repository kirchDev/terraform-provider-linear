resource "linear_release_stage" "canary" {
  pipeline_id = linear_release_pipeline.web.id
  name        = "Canary"
  color       = "#f2c94c"
  type        = "started"
  position    = 1
}

resource "linear_release_stage" "production" {
  pipeline_id = linear_release_pipeline.web.id
  name        = "Production"
  color       = "#4cb782"
  type        = "completed"
  position    = 2
}
