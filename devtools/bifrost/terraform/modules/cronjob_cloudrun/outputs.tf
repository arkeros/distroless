output "service_account_email" {
  value       = google_service_account.runtime.email
  description = "Runtime GSA email for the Cloud Run Job. Grant IAM to this for GCP resource access."
}

output "scheduler_service_account_email" {
  value       = google_service_account.scheduler.email
  description = "Cloud Scheduler invoker GSA email."
}

output "job_id" {
  value       = google_cloud_run_v2_job.this.id
  description = "Cloud Run Job resource ID."
}
