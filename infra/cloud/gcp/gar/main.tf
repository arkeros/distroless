provider "google" {
  project = var.project_id
}

# Artifact Registry API has to be enabled before we can create repositories in
# the project. Managed here so a fresh project bootstraps in a single apply
# instead of requiring an out-of-band `gcloud services enable`.

resource "google_project_service" "artifactregistry" {
  project = var.project_id
  service = "artifactregistry.googleapis.com"

  # Leaves the API enabled even if this root is destroyed. Disabling an API on
  # a project in active use is never what we want.
  disable_on_destroy = false
}

# Single multi-region repo in `europe`. All Senku-built images live here;
# consumers (Cloud Run in any region, K8s clusters, local `docker pull`) pull
# from `europe-docker.pkg.dev/<project>/containers/...`.
#
# Multi-region vs. per-Cloud-Run-region fan-out: a regional fan-out buys at
# most ~1s of cold-start pull latency for far-from-EU Cloud Run regions, at
# the cost of making every release a 5-way push-consistency problem. Cold
# starts aren't in the request-path SLO, so not worth it. If cold-start
# latency ever needs to drop to zero for a specific service, raise its Cloud
# Run `scaling.min` instead — that kills the cold-start class entirely rather
# than shaving a second off it.
#
# `europe` (not `us`) because the team and most traffic are EU-centric.

resource "google_artifact_registry_repository" "containers" {
  project       = var.project_id
  location      = "europe"
  repository_id = "containers"
  format        = "DOCKER"
  description   = "Private container images for Senku workloads (deploy-time pulls by Cloud Run, K8s, etc.)."

  depends_on = [google_project_service.artifactregistry]
}
