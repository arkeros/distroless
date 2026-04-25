output "service_account_email" {
  value       = local.service_account_email
  description = "Runtime GSA email — either the module-created SA (default) or the external `var.service_account_email` if one was provided. Grant IAM on other resources (DBs, Redis, Secret Manager secrets) to this."
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

output "custom_domain_dns_records" {
  value       = { for d, m in google_cloud_run_domain_mapping.custom : d => m.status[0].resource_records }
  description = "DNS records (type, name, rrdata) Cloud Run expects for each custom domain. Create these in your DNS provider to finish the mapping — Google won't serve traffic on the domain until they resolve."
}
