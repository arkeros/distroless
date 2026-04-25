output "repository_url" {
  value       = "${google_artifact_registry_repository.containers.location}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.containers.repository_id}"
  description = "Pull/push host + repo path. Prepend to an image name for a full reference, e.g. `<repository_url>/registry@sha256:<digest>`."
}
