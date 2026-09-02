output "repository_url" {
  description = "Pull/push host + repo path. Prepend to an image name for a full reference, e.g. `<repository_url>/registry@sha256:<digest>`."
  value       = "${google_artifact_registry_repository.containers.location}-docker.pkg.dev/${local.project}/${google_artifact_registry_repository.containers.repository_id}"
}

output "registry_service_account_email" {
  description = "Runtime GSA the registry Cloud Run service runs as. Grant downstream IAM (pull credentials, Secret Manager) here once — every region picks it up."
  value       = google_service_account.registry.email
}

output "github_workload_identity_provider" {
  description = "Full resource name for `google-github-actions/auth`'s `workload_identity_provider` input."
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "github_service_account" {
  description = "Email for `google-github-actions/auth`'s `service_account` input."
  value       = google_service_account.github_actions.email
}

output "github_cache_service_account" {
  description = "Email for `google-github-actions/auth`'s `service_account` input in `main`-only jobs that read and write the Bazel remote cache (ci.yaml, under `environment: prod`)."
  value       = google_service_account.github_actions_cache.email
}

output "github_cache_readonly_service_account" {
  description = "Email for `google-github-actions/auth`'s `service_account` input in jobs that may run off `main` and only read the Bazel remote cache (pr.yaml, the update-* workflows)."
  value       = google_service_account.github_actions_cache_ro.email
}

output "bazel_remote_cache_url" {
  description = "Value for Bazel's `--remote_cache`. Must match `common:gcs --remote_cache` in //.bazelrc."
  value       = "https://storage.googleapis.com/${google_storage_bucket.bazel_cache.name}"
}
