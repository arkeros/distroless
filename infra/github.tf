# Workload Identity Federation for this repo's GitHub Actions.
#
# The `github` pool in this project belongs to arkeros/senku, so this repo gets
# its own pool rather than sharing one it does not own — pools are cheap, and a
# shared one would mean senku's infra could widen or revoke this repo's access.

data "google_project" "this" {
  project_id = local.project
}

locals {
  github_repository = "arkeros/distroless"
}

resource "google_iam_workload_identity_pool" "github" {
  project                   = local.project
  workload_identity_pool_id = "github-distroless"
  display_name              = "GitHub Actions (distroless)"
  description               = "Identity pool for ${local.github_repository} workflows."
}

# `attribute_condition` is the outer gate: no token from any other repository
# can even mint a credential in this pool, whatever a service account binding
# below might say. Keep it here rather than relying on binding scope alone —
# this is the check that cannot be widened by editing an IAM member.
resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = local.project
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"
  display_name                       = "GitHub OIDC"
  attribute_condition                = "assertion.repository == '${local.github_repository}'"

  attribute_mapping = {
    "google.subject"             = "assertion.sub"
    "attribute.repository"       = "assertion.repository"
    "attribute.repository_owner" = "assertion.repository_owner"
    "attribute.environment"      = "assertion.environment"
    "attribute.ref"              = "assertion.ref"
  }

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

# The identity CI pushes and deploys as.
resource "google_service_account" "github_actions" {
  project      = local.project
  account_id   = "github-actions-distroless"
  display_name = "GitHub Actions deploy identity for ${local.github_repository}"
}

# Keyed on `attribute.environment/prod`, not on the repository: the provider
# condition above already pins the repository, so what this adds is the
# environment gate. `prod`'s deployment branch policy names `main` and nothing
# else, and GitHub validates it *before* minting the OIDC token — so the branch
# check lives in the identity layer rather than in workflow YAML any committer
# can edit. That environment, and the rulesets that decide what may reach
# `main` in the first place, are in repo.tf.
resource "google_service_account_iam_member" "github_actions_wif" {
  service_account_id = google_service_account.github_actions.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/projects/${data.google_project.this.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.github.workload_identity_pool_id}/attribute.environment/prod"
}

# Push images. Scoped to the one repository, not project-wide.
resource "google_artifact_registry_repository_iam_member" "github_actions_writer" {
  project    = local.project
  location   = google_artifact_registry_repository.containers.location
  repository = google_artifact_registry_repository.containers.name
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.github_actions.email}"
}

# Deploy revisions. Project-scoped because a service in a not-yet-deployed
# region does not exist to be bound to — a per-service binding cannot create
# the first revision in a new region.
resource "google_project_iam_member" "github_actions_run_admin" {
  project = local.project
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.github_actions.email}"
}

# Deploying a service that *runs as* svc-registry means acting as it, which is
# a distinct permission from being able to deploy at all. Scoped to that one
# account, so this identity cannot borrow any other.
resource "google_service_account_iam_member" "github_actions_act_as_registry" {
  service_account_id = google_service_account.registry.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_actions.email}"
}

# The same for the directory's runtime identity, and the reason it is a second
# resource rather than a wider grant: `roles/iam.serviceAccountUser` at the
# project level would let this identity act as *every* account in senku-prod,
# including the two it deploys nothing as.
#
# One binding per runtime account is a list that has to be extended whenever a
# service is added, and it has already been missed once: `svc-web` was created
# with its Artifact Registry reader binding, the deploy job was written, and
# this grant was not — so every `Deploy web` run failed with
# `iam.serviceaccounts.actAs` denied while `registry` deployed fine, and all
# three web regions kept serving the placeholder image. Adding a third service
# means adding a third binding here.
resource "google_service_account_iam_member" "github_actions_act_as_web" {
  service_account_id = google_service_account.web.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_actions.email}"
}
