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

# Two cache identities, split by what they may do to the bucket, because the
# cache is on the path to a signed image.
#
# The mirror's provenance is written by the build platform, not by the build
# (ADR 0014). What that proves is "this digest came out of run N of ci.yaml on
# `main`" — it says nothing about what run N *read*. If any branch could write
# an action result that `main` later takes as a cache hit, a pull request could
# put bytes into an image the platform then vouches for. So writes are
# `main`-only, and everything else is a viewer.

# The writer. Distinct from `github_actions` in github.tf, which can also ship
# a Cloud Run revision; this one reaches the bucket and nothing else, so the
# jobs that only need a warm cache — `test`, `publish`, `release` — never hold
# the deploy identity.
resource "google_service_account" "github_actions_cache" {
  project      = local.project
  account_id   = "github-actions-cache"
  display_name = "GitHub Actions Bazel cache writer for ${local.github_repository}"
}

# Bound to `attribute.environment/prod`, exactly like the deploy account. The
# `prod` environment's deployment branch policy (repo.tf) names `main` and
# nothing else, and GitHub validates it *before* minting the token, so no job
# off `main` can hold this identity no matter what its YAML says. A ref binding
# (`attribute.ref/refs/heads/main`) would be equally strong against this
# threat; the environment is used because it is the pattern the deploy account
# already established, it is Terraform-managed, and every write-capable run
# shows up in the repository's deployments log.
resource "google_service_account_iam_member" "github_actions_cache_wif" {
  service_account_id = google_service_account.github_actions_cache.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/projects/${data.google_project.this.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.github.workload_identity_pool_id}/attribute.environment/prod"
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

# The reader, for pull requests (pr.yaml). Keyed on the repository, so any
# branch in it can mint this credential — the pool provider still pins
# `assertion.repository`, and GitHub withholds `id-token: write` from pull
# requests opened off a fork, which then run cacheless. What it can do with the
# credential is read: a viewer cannot poison anything `main` will trust.
resource "google_service_account" "github_actions_cache_ro" {
  project      = local.project
  account_id   = "github-actions-cache-ro"
  display_name = "GitHub Actions Bazel cache reader for ${local.github_repository}"
}

resource "google_service_account_iam_member" "github_actions_cache_ro_wif" {
  service_account_id = google_service_account.github_actions_cache_ro.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/projects/${data.google_project.this.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.github.workload_identity_pool_id}/attribute.repository/${local.github_repository}"
}

# `--config=gcs-readonly` in //.bazelrc pairs with this: Bazel is told not to
# upload, so the 403 this role would answer an upload with is never provoked.
resource "google_storage_bucket_iam_member" "github_actions_cache_ro" {
  bucket = google_storage_bucket.bazel_cache.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.github_actions_cache_ro.email}"
}

# The deploy job authenticates as `github_actions` for Artifact Registry and
# Cloud Run, and one job gets one set of application-default credentials — so
# that account needs cache access too, or the deploy build runs cold. It is
# only reachable behind the same `prod` environment as the writer above, so
# this widens nothing.
resource "google_storage_bucket_iam_member" "github_actions_cache_rw_deploy" {
  bucket = google_storage_bucket.bazel_cache.name
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.github_actions.email}"
}
