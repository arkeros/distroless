# Bazel remote cache.
#
# Cache only — there is no remote *execution* here. Actions run on whatever
# machine invoked Bazel (a dev laptop, a GitHub runner); all that is shared is
# the action result store, over GCS's plain HTTP API via `--config=gcs`.
#
# `US`, not `europe` like the Artifact Registry repo: this bucket's traffic is
# almost entirely GitHub-hosted runners, which run on Azure in the US. A cache
# hit is a round trip, and there are thousands of them per build, so co-locating
# with the runners is worth more than co-locating with the EU developers who
# reach it occasionally via `--config=gcs`.
#
# Multi-region rather than a single US region because GitHub does not pin where
# a runner lands — their published ranges span East US 2 and Central US — so
# there is no one region to match. This tracks the fleet wherever it sits.
resource "google_storage_bucket" "bazel_cache" {
  project  = local.project
  name     = "senku-prod-bazel-cache"
  location = "US"

  uniform_bucket_level_access = true

  # A Bazel cache is pure derived data: every entry can be recomputed, and a
  # miss costs one rebuild. Nothing in here is worth retaining after a delete,
  # and the default 7-day soft-delete window would bill us for a rolling week
  # of a workload whose entire character is high churn.
  soft_delete_policy {
    retention_duration_seconds = 0
  }

  # Unbounded growth is the failure mode of every remote cache. Age, not
  # last-access, is the only condition GCS lifecycle offers; 30 days is long
  # enough that a still-hot entry rarely expires, and a miss just re-uploads.
  lifecycle_rule {
    condition {
      age = 30
    }
    action {
      type = "Delete"
    }
  }

  # Abandoned resumable uploads are invisible to the age rule above until they
  # complete, which they never do.
  lifecycle_rule {
    condition {
      days_since_noncurrent_time = 1
    }
    action {
      type = "Delete"
    }
  }
}

# A second CI identity, distinct from `github_actions` in github.tf.
#
# The deploy identity is deliberately reachable only from the `prod`
# environment, which GitHub gates on `main` + reviewers. Cache access has to be
# available to every push and pull request or it buys nothing, so it cannot
# hang off that binding — and widening the deploy account to match would hand
# every branch the ability to ship a Cloud Run revision. Two accounts, two
# blast radii.
resource "google_service_account" "github_actions_cache" {
  project      = local.project
  account_id   = "github-actions-cache"
  display_name = "GitHub Actions Bazel cache identity for ${local.github_repository}"
}

# Keyed on the repository, so any branch or pull request in it can mint this
# credential. That is a wider gate than the deploy account's, and intentionally
# so: the pool provider still pins `assertion.repository`, and GitHub withholds
# `id-token: write` from pull requests opened off a fork. Whoever can write to
# the cache is therefore whoever can push a branch here — the same set of people
# who can already land code on `main`.
resource "google_service_account_iam_member" "github_actions_cache_wif" {
  service_account_id = google_service_account.github_actions_cache.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/projects/${data.google_project.this.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.github.workload_identity_pool_id}/attribute.repository/${local.github_repository}"
}

# `objectUser` rather than creator+viewer: Bazel PUTs action-cache entries
# unconditionally, and overwriting an existing object in GCS requires
# `storage.objects.delete`. Scoped to this bucket, so it reaches nothing else in
# the project.
resource "google_storage_bucket_iam_member" "github_actions_cache_rw" {
  bucket = google_storage_bucket.bazel_cache.name
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.github_actions_cache.email}"
}

# The deploy job authenticates as `github_actions` for Artifact Registry and
# Cloud Run, and one job gets one set of application-default credentials — so
# that account needs cache access too, or the deploy build runs cold.
resource "google_storage_bucket_iam_member" "github_actions_cache_rw_deploy" {
  bucket = google_storage_bucket.bazel_cache.name
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.github_actions.email}"
}
