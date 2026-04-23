output "service_account_email" {
  value       = google_service_account.runtime.email
  description = "Runtime GSA email. Grant IAM on other resources (DBs, Redis, Secret Manager secrets) to this."
}

output "service_name" {
  value       = google_cloud_run_v2_service.this.name
  description = "Cloud Run service name."
}

output "service_uri" {
  value       = google_cloud_run_v2_service.this.uri
  description = "Canonical HTTPS URL Cloud Run assigned to this service. Useful for wiring a Cloud Scheduler / Cloud Tasks target or a load balancer backend."
}

output "service_id" {
  value       = google_cloud_run_v2_service.this.id
  description = "Full GCP resource ID. Use for IAM bindings from other Terraform."
}
