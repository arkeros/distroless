locals {
  runtime_sa_id   = coalesce(var.service_account_id, "crj-${var.name}")
  labels          = merge({ "app.kubernetes.io/name" = var.name }, var.labels)
  cpu_quantity    = "${floor(var.resources.cpu * 1000)}m"
  memory_quantity = "${var.resources.memory}Mi"
  field_manager   = "terraform"

  # 13 hex chars = 52 bits: collision-safe and fits in float64 so the same
  # value feeds data_wo_revision as an int.
  secret_env_hash = length(var.secret_env) > 0 ? substr(sha256(jsonencode(var.secret_env)), 0, 13) : ""
  secret_env_name = length(var.secret_env) > 0 ? "${var.name}-env-${local.secret_env_hash}" : ""

  # Hardcoded "app" matches the Go renderer and keeps the container name
  # stable across workloads (useful for any per-container policy refs).
  container_name = "app"

  container_security_context = {
    runAsNonRoot             = true
    allowPrivilegeEscalation = false
    readOnlyRootFilesystem   = true
    capabilities             = { drop = ["ALL"] }
    seccompProfile           = { type = "RuntimeDefault" }
  }

  pod_security_context = {
    runAsNonRoot   = true
    seccompProfile = { type = "RuntimeDefault" }
  }

  container = merge(
    {
      name            = local.container_name
      image           = var.image
      args            = var.args
      securityContext = local.container_security_context
      # Batch gets BOTH CPU and memory request==limit. The "CPU request only"
      # rule is a web-serving rule; a runaway batch job with no CPU limit
      # would starve co-tenant pods on the node.
      resources = {
        requests = {
          cpu    = local.cpu_quantity
          memory = local.memory_quantity
        }
        limits = {
          cpu    = local.cpu_quantity
          memory = local.memory_quantity
        }
      }
      env = [
        for k, v in var.env : {
          name  = k
          value = v
        }
      ]
    },
    length(var.secret_env) > 0 ? {
      envFrom = [{ secretRef = { name = local.secret_env_name } }]
    } : {},
  )
}

# ─── GCP identity ────────────────────────────────────────────────────────────

resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = local.runtime_sa_id
  display_name = "Runtime identity for cronjob ${var.name}"
}

resource "google_service_account_iam_member" "workload_identity" {
  service_account_id = google_service_account.runtime.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[${var.namespace}/${var.name}]"
}

# ─── Kubernetes via SSA ──────────────────────────────────────────────────────

resource "kubernetes_manifest" "service_account" {
  field_manager {
    name            = local.field_manager
    force_conflicts = true
  }

  manifest = {
    apiVersion = "v1"
    kind       = "ServiceAccount"
    metadata = {
      name        = var.name
      namespace   = var.namespace
      labels      = local.labels
      annotations = { "iam.gke.io/gcp-service-account" = google_service_account.runtime.email }
    }
  }
}

ephemeral "google_secret_manager_secret_version" "env" {
  for_each = var.secret_env
  project  = each.value.project
  secret   = each.value.secret
  version  = each.value.version
}

# Typed-resource carve-out. See service_kubernetes/main.tf for the full rationale:
# kubernetes_manifest doesn't yet expose write-only on its dynamic `manifest`
# attribute, so ephemeral secret material can't flow through SSA. Single-
# writer → losing SSA coordination here costs nothing.
resource "kubernetes_secret_v1" "env" {
  count = length(var.secret_env) > 0 ? 1 : 0

  metadata {
    name      = local.secret_env_name
    namespace = var.namespace
    labels    = local.labels
  }

  data_wo = {
    for k, _ in var.secret_env :
    k => ephemeral.google_secret_manager_secret_version.env[k].secret_data
  }
  # See service_kubernetes/main.tf for the non-monotonic revision rationale.
  data_wo_revision = parseint(local.secret_env_hash, 16)

  lifecycle {
    create_before_destroy = true
  }
}

resource "kubernetes_manifest" "cron_job" {
  field_manager {
    name            = local.field_manager
    force_conflicts = false
  }

  # Image version is owned by the push controller (Flagger / Argo) on
  # subsequent applies; Terraform provides the initial value.
  computed_fields = [
    "metadata.annotations",
    "metadata.labels",
    "spec.jobTemplate.spec.template.spec.containers[0].image",
  ]

  manifest = {
    apiVersion = "batch/v1"
    kind       = "CronJob"
    metadata = {
      name      = var.name
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      schedule                   = var.schedule.cron
      timeZone                   = var.schedule.time_zone
      concurrencyPolicy          = "Forbid"
      successfulJobsHistoryLimit = 3
      failedJobsHistoryLimit     = 1
      jobTemplate = {
        metadata = { labels = local.labels }
        spec = {
          parallelism           = var.job.parallelism
          completions           = var.job.completions
          backoffLimit          = var.job.max_retries
          activeDeadlineSeconds = var.job.timeout_seconds
          template = {
            metadata = { labels = local.labels }
            spec = {
              serviceAccountName = var.name
              restartPolicy      = "Never"
              securityContext    = local.pod_security_context
              containers         = [local.container]
            }
          }
        }
      }
    }
  }

  # See service_kubernetes/main.tf for why Secret is an explicit depends_on.
  depends_on = [
    kubernetes_manifest.service_account,
    kubernetes_secret_v1.env,
  ]
}
