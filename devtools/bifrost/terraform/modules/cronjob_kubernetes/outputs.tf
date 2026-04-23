output "service_account_email" {
  value       = google_service_account.runtime.email
  description = "Runtime GSA email. Grant IAM to this for GCP resource access from the job."
}

output "kubernetes_service_account_name" {
  value       = kubernetes_manifest.service_account.manifest.metadata.name
  description = "Kubernetes ServiceAccount name."
}

output "cron_job_name" {
  value       = kubernetes_manifest.cron_job.manifest.metadata.name
  description = "CronJob name."
}
