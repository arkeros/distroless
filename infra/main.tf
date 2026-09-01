provider "google" {
  project = local.project
}

# No `token`: the provider falls back to `gh auth token`, so this root stays
# applicable by hand with the `gh` login the operator already has, and there is
# no long-lived PAT to store or rotate for it. See repo.tf for what it manages.
provider "github" {
  owner = local.github_owner
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

  # The regions the registry is deployed to, read from the same file the
  # deploy workflow builds its matrix from — adding a region there gives it an
  # invoker binding here without a second edit.
  registry_regions = jsondecode(file("${path.module}/../oci/cmd/registry/regions.json"))
  registry_service = "registry"

  # Same arrangement for the directory site. It shares the load balancer with
  # the registry — the URL map routes `/v2/*` to `registry` and `/*` here —
  # but it is a separate Cloud Run service with its own identity, so a bug in
  # the page renderer cannot borrow the registry's credentials.
  web_regions = jsondecode(file("${path.module}/../web/cmd/server/regions.json"))
  web_service = "web"
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
  description   = "Private container images for Distroless workloads (deploy-time pulls by Cloud Run)."

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

# The service shells. Terraform owns that they exist and what ingress they
# accept; CI owns every revision — the deploy replaces the template from the
# Knative manifest with a digest-pinned image, and `ignore_changes` stops the
# next apply reverting it.
#
# The shell exists to dissolve an ordering problem, not to manage Cloud Run.
# `google_cloud_run_v2_service_iam_member` binds to a service by name, and
# Terraform cannot `depends_on` something another system creates — so a
# brand-new service or region needed an apply, a deploy, then a second apply.
# Owning the shell turns that into an ordinary reference.
#
# `ingress` is deliberately *not* ignored. It is what keeps the services off
# the open internet — the `allUsers` binding below is only safe behind it — so
# it stays managed and drift shows up as a diff. The Knative manifests declare
# it too, and must: `gcloud run services replace` is a whole-object replace, so
# a manifest omitting it would reset the service to public on every deploy.
#
# These predate the arrangement, so they are adopted rather than created. The
# `import` block is the whole point — without it Terraform would try to create
# services that already serve distroless.io and fail on the conflict. Nothing
# here forces replacement (`name`, `location` and `project` are the only
# replace-triggering fields and all three match), so adoption is a state
# operation, not a rollout.
resource "google_cloud_run_v2_service" "registry" {
  for_each = toset(local.registry_regions)

  project             = local.project
  location            = each.value
  name                = local.registry_service
  ingress             = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
  deletion_protection = true

  # Never actually applied to these: they exist already, and the template is
  # ignored from the moment they are imported. It is here because the schema
  # requires one.
  template {
    service_account = google_service_account.registry.email

    containers {
      image = "us-docker.pkg.dev/cloudrun/container/hello"
    }
  }

  lifecycle {
    ignore_changes = [
      template,
      traffic,
      labels,
      annotations,
      client,
      client_version,
    ]
  }
}

import {
  for_each = toset(local.registry_regions)

  to = google_cloud_run_v2_service.registry[each.value]
  id = "projects/${local.project}/locations/${each.value}/services/${local.registry_service}"
}

# The registry Cloud Run services accept anonymous invokes: the external HTTPS
# load balancer's Serverless NEG calls them without injecting an OIDC identity,
# so `allUsers` needs `run.invoker`. What keeps the services off the open
# internet is `ingress` above, not this binding.
#
# This is durable policy: it does not change when a new revision ships, which
# is why it lives here rather than in the deploy job.
resource "google_cloud_run_v2_service_iam_member" "registry_public" {
  for_each = google_cloud_run_v2_service.registry

  project  = local.project
  location = each.value.location
  name     = each.value.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Runtime identity for the directory site, shared across every region. Separate
# from `svc-registry` on purpose: the two services do different jobs and should
# be distinguishable in audit logs, even though today both only pull images.
#
# As with the registry, the service itself is not Terraform-managed — it is
# deployed from //web/cmd/server:service.yaml by CI. This account is
# infrastructure that deploy consumes, so it is owned here.
resource "google_service_account" "web" {
  project      = local.project
  account_id   = "svc-web"
  display_name = "Runtime identity for web (shared across all regions)"
}

# Cloud Run pulls the image as its runtime identity, so that identity needs
# read access to the repo it pulls from.
resource "google_artifact_registry_repository_iam_member" "web_reader" {
  project    = local.project
  location   = google_artifact_registry_repository.containers.location
  repository = google_artifact_registry_repository.containers.name
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.web.email}"
}

# Same arrangement as `registry` above, minus the adoption: these services do
# not exist yet, so Terraform creates them and the first deploy replaces the
# placeholder template.
resource "google_cloud_run_v2_service" "web" {
  for_each = toset(local.web_regions)

  project             = local.project
  location            = each.value
  name                = local.web_service
  ingress             = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
  deletion_protection = true

  # Only ever used to bring the service into existence; the first deploy
  # replaces it. Google's public sample rather than one of ours, so a service
  # that somehow never got deployed is obviously not us, instead of quietly
  # serving a stale build that looks like us.
  template {
    service_account = google_service_account.web.email

    containers {
      image = "us-docker.pkg.dev/cloudrun/container/hello"
    }
  }

  lifecycle {
    ignore_changes = [
      template,
      traffic,
      labels,
      annotations,
      client,
      client_version,
    ]
  }
}

# Anonymous invokes, for the same reason as `registry_public`: the LB's
# Serverless NEG calls without injecting an OIDC identity. What keeps the
# service off the open internet is `ingress` above, not this binding.
#
# Keyed off the service resource rather than off the region list, so the
# dependency is a reference Terraform can see rather than something an operator
# has to know.
resource "google_cloud_run_v2_service_iam_member" "web_public" {
  for_each = google_cloud_run_v2_service.web

  project  = local.project
  location = each.value.location
  name     = each.value.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
