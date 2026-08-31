output "repository_url" {
  description = "Pull/push host + repo path. Prepend to an image name for a full reference, e.g. `<repository_url>/registry@sha256:<digest>`."
  value       = "${google_artifact_registry_repository.containers.location}-docker.pkg.dev/${local.project}/${google_artifact_registry_repository.containers.repository_id}"
}

output "registry_service_account_email" {
  description = "Runtime GSA the registry Cloud Run service runs as. Grant downstream IAM (pull credentials, Secret Manager) here once — every region picks it up."
  value       = google_service_account.registry.email
}
