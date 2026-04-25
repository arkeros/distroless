output "lb_backends" {
  value = {
    registry = {
      service_name = "registry"
      regions      = var.regions
      paths        = ["/v2/*"]
    }
  }
  description = "LB backends exposed for `infra/cloud/gcp/lb` to read via `terraform_remote_state`. The single `registry` backend fans out across every deploy region — one NEG per region, all attached to one backend service, so the LB picks the closest healthy region per request."
}

output "service_account_email" {
  value       = google_service_account.registry.email
  description = "Runtime GSA shared across every regional Cloud Run service. Grant downstream IAM (e.g. pull credentials, Secret Manager) here once — every region picks it up."
}

output "service_uris" {
  value       = { for region, m in module.registry : region => m.service_uri }
  description = "Cloud Run `*.run.app` URL per region. Not reachable directly because of `ingress = INTERNAL_LOAD_BALANCER`; exposed for `gcloud` smoke tests and debugging."
}
