provider "google" {
  project = local.project
}

locals {
  project = "senku-prod"

  # `europe` (not `us`) because the team and most traffic are EU-centric.
  #
  # Multi-region vs. per-Cloud-Run-region fan-out: regional fan-out buys at
  # most ~1s of cold-start pull latency for far-from-EU Cloud Run regions, at
  # the cost of making every release a 5-way push-consistency problem. Cold
  # starts aren't in the request-path SLO, so not worth it. If cold-start
  # latency ever needs to drop to zero for a specific service, raise that
  # service's minScale instead — that kills the cold-start class entirely
  # rather than shaving a second off it.
  location      = "europe"
  repository_id = "containers"
}

# Artifact Registry API has to be enabled before we can create repositories in
# the project. Managed here so a fresh project bootstraps in a single apply
# instead of requiring an out-of-band `gcloud services enable`.
#
# `disable_on_destroy = false` leaves the API enabled even if this root is
# destroyed — disabling an API on a project in active use is never what we want.
resource "google_project_service" "artifactregistry" {
  project            = local.project
  service            = "artifactregistry.googleapis.com"
  disable_on_destroy = false
}

# Single multi-region repo. All images built here live in it; consumers (Cloud
# Run in any region, local `docker pull`) read from
# `<location>-docker.pkg.dev/<project>/<repository_id>/...`.
resource "google_artifact_registry_repository" "containers" {
  project       = local.project
  location      = local.location
  repository_id = local.repository_id
  format        = "DOCKER"
  description   = "Private container images for Senku workloads (deploy-time pulls by Cloud Run)."

  depends_on = [google_project_service.artifactregistry]
}

# Runtime identity for the registry Cloud Run service, shared across every
# region: one principal in audit logs and IAM bindings, not three.
#
# The service itself is not Terraform-managed — it is deployed from the Knative
# manifest at `//oci/cmd/registry:service.yaml` by the `deploy` job in
# `.github/workflows/ci.yaml`. This account is infrastructure the deploy
# consumes, so it is owned here rather than by the thing that references it.
resource "google_service_account" "registry" {
  project      = local.project
  account_id   = "svc-registry"
  display_name = "Runtime identity for registry (shared across all regions)"
}

# Cloud Run pulls the image as its runtime identity, so that identity needs
# read access to the repo it pulls from.
resource "google_artifact_registry_repository_iam_member" "registry_reader" {
  project    = local.project
  location   = google_artifact_registry_repository.containers.location
  repository = google_artifact_registry_repository.containers.name
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.registry.email}"
}
