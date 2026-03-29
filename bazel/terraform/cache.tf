resource "google_storage_bucket" "bazel_cache" {
  name                        = "bazel-senku-remote-cache"
  project                     = var.project_id
  location                    = var.region
  uniform_bucket_level_access = true

  lifecycle_rule {
    condition {
      age = 30
    }
    action {
      type = "Delete"
    }
  }
}
