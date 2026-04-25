provider "google" {
  project = var.project_id
}

# One Cloud Run service per region, all sharing the same logical name
# (`registry`) and the same runtime GSA. Region fan-out is for latency, not
# for identity — the five services are replicas of the same workload, so
# they share one SA (one row in audit logs / IAM bindings, not five).
# Cloud Run service names are region-scoped, so `registry` in every region
# does not collide. Fronting them behind a single anycast IP is the job of
# the LB stack (see `infra/cloud/gcp/lb`), not this root.

resource "google_service_account" "registry" {
  project      = var.project_id
  account_id   = "svc-registry"
  display_name = "Runtime identity for registry (shared across all regions)"
}

module "registry" {
  for_each = var.regions

  source = "../../../../devtools/bifrost/terraform/modules/service_cloudrun"

  name       = "registry"
  project_id = var.project_id
  region     = each.value

  # Share one identity across regions — the module skips creating its own SA
  # when `service_account_email` is set.
  service_account_email = google_service_account.registry.email

  # Image URI (registry + repo + digest) comes through `var.image`, populated
  # by `image.auto.tfvars.json` — see variables.tf.
  image     = var.image
  resources = { cpu = 1, memory = 512 }
  scaling   = { min = 0, max = 3 }

  args = [
    "--upstream=${var.upstream}",
    "--repository-prefix=${var.repository_prefix}",
  ]

  probes = {
    startup_path  = "/v2/"
    liveness_path = "/v2/"
  }

  # Ingress locked to LB-only. Org policy on senku-prod forbids
  # `INGRESS_TRAFFIC_ALL`, and architecturally the registry sits behind the
  # shared external HTTPS LB (`infra/cloud/gcp/lb`) anyway. `public = true`
  # stays because the LB's Serverless NEG calls without injecting an OIDC
  # identity, so the services must accept anonymous invokes — the ingress
  # filter is what keeps the service off the open internet.
  ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
  public  = true
}
