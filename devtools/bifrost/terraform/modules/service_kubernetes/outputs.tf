output "service_account_email" {
  value       = google_service_account.runtime.email
  description = "GSA email for the runtime identity. Grant IAM on other resources (DB, Redis, buckets) to this."
}

output "kubernetes_service_account_name" {
  value       = kubernetes_manifest.service_account.manifest.metadata.name
  description = "Kubernetes ServiceAccount name."
}

output "deployment_name" {
  value       = kubernetes_manifest.deployment.manifest.metadata.name
  description = "Deployment name. Consumers: Flagger Canary.spec.targetRef, HPA ScaleTargetRef wiring from external tools."
}

output "service_name" {
  value       = kubernetes_manifest.service.manifest.metadata.name
  description = "Service name, for DNS / ingress / Flagger traffic routing."
}
