output "wif_provider" {
  description = "Workload Identity Federation provider"
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "sa_email" {
  description = "GitHub Actions service account email"
  value       = google_service_account.github_actions.email
}
