locals {
  service_account_id = coalesce(var.service_account_id, "svc-${var.name}")
  gsa_email          = "${local.service_account_id}@${var.project_id}.iam.gserviceaccount.com"
  labels             = merge({ "app.kubernetes.io/name" = var.name }, var.labels)
  selector_labels    = { "app.kubernetes.io/name" = var.name }
  cpu_request        = "${floor(var.resources.cpu * 1000)}m"
  memory_quantity    = "${var.resources.memory}Mi"
  field_manager      = "terraform"

  # PushOps/MPM-style immutable naming: the Secret's name carries a content
  # hash, so any change to var.secret_env produces a new Secret object and a
  # new ReplicaSet. Rollback = revert the inputs, name hashes back to the
  # prior value, Secret is re-materialised from Secret Manager.
  #
  # 13 hex chars = 52 bits: collision-safe at scale (~1 in 4.5 quadrillion
  # per pair) AND fits in Terraform's float64 number type so the same value
  # can feed data_wo_revision as an int.
  secret_env_hash = length(var.secret_env) > 0 ? substr(sha256(jsonencode(var.secret_env)), 0, 13) : ""
  secret_env_name = length(var.secret_env) > 0 ? "${var.name}-env-${local.secret_env_hash}" : ""

  # Container is named "app" regardless of workload name so VPA
  # resourcePolicy can target it by a stable name.
  container_name = "app"

  # Hardened security context applied to every container. Mirrors the Go
  # renderer's defaults — restrictive by construction, not opt-in.
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

  # Container spec is assembled via merge so optional fields (envFrom, probes)
  # only appear when set — kubernetes_manifest rejects null fields.
  container = merge(
    {
      name            = local.container_name
      image           = var.image
      args            = var.args
      ports           = [{ containerPort = var.port }]
      securityContext = local.container_security_context
      env = [
        for k, v in merge({ PORT = tostring(var.port) }, var.env) : {
          name  = k
          value = v
        }
      ]
      resources = {
        requests = {
          cpu    = local.cpu_request
          memory = local.memory_quantity
        }
        limits = {
          memory = local.memory_quantity
        }
      }
    },
    length(var.secret_env) > 0 ? {
      envFrom = [{ secretRef = { name = local.secret_env_name } }]
    } : {},
    try(var.probes.startup_path, null) != null ? {
      startupProbe = { httpGet = { path = var.probes.startup_path, port = var.port } }
    } : {},
    try(var.probes.liveness_path, null) != null ? {
      livenessProbe = { httpGet = { path = var.probes.liveness_path, port = var.port } }
    } : {},
    try(var.probes.readiness_path, null) != null ? {
      readinessProbe = { httpGet = { path = var.probes.readiness_path, port = var.port } }
    } : {},
  )

  pdb_enabled = var.autoscaling.min >= 2
}

# ─── GCP identity (google provider, no SSA concern) ──────────────────────────

resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = local.service_account_id
  display_name = "Runtime identity for ${var.name}"
}

resource "google_service_account_iam_member" "workload_identity" {
  service_account_id = google_service_account.runtime.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[${var.namespace}/${var.name}]"
}

# ─── Kubernetes objects via Server-Side Apply ────────────────────────────────
#
# Field-manager coordination:
#   • "terraform" (this module) owns everything it declares.
#   • Flagger / Argo Rollouts owns spec.replicas and the container image
#     during rollout. Those fields are listed in computed_fields so Terraform
#     does not send them under its field manager after initial creation and
#     therefore does not fight the rollout controller.
#
# SSA cost: `terraform plan` opens a connection to the apiserver to dry-run
# each manifest against the live schema. Plans fail on an unreachable cluster.

resource "kubernetes_manifest" "service_account" {
  field_manager {
    name            = local.field_manager
    force_conflicts = true
  }

  manifest = {
    apiVersion = "v1"
    kind       = "ServiceAccount"
    metadata = {
      name      = var.name
      namespace = var.namespace
      labels    = local.labels
      annotations = {
        "iam.gke.io/gcp-service-account" = google_service_account.runtime.email
      }
    }
  }
}

ephemeral "google_secret_manager_secret_version" "env" {
  for_each = var.secret_env
  project  = each.value.project
  secret   = each.value.secret
  version  = each.value.version
}

# Typed-resource carve-out. Every other K8s object in this module flows
# through kubernetes_manifest under SSA; this one doesn't. Reason:
# kubernetes_manifest does not expose write-only on its dynamic `manifest`
# attribute, and ephemeral values cannot flow into a non-write-only
# destination. The hashicorp/kubernetes provider hasn't shipped a
# `manifest_wo` sidecar or per-path write-only annotations yet.
#
# Practical cost: the Secret is client-side applied rather than SSA. Since
# only this module writes to it (single-writer), losing SSA coordination
# here costs nothing. When the provider gains write-only manifest support,
# flip this resource to kubernetes_manifest with the ephemeral value routed
# to the write-only surface. Tracking: hashicorp/terraform-provider-kubernetes.
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
  # Same content hash used in metadata.name, re-parsed as an int. The
  # provider compares revision by inequality (!= prior), not strict
  # monotonic (> prior), so a content change producing a numerically
  # smaller int is fine. If a future provider version enforces strict
  # monotonicity, flip this to a time_static-backed source.
  data_wo_revision = parseint(local.secret_env_hash, 16)

  # Replacement happens when local.secret_env_name (the hash-suffixed
  # metadata.name) changes. create_before_destroy ensures the new Secret
  # exists before the old one is deleted, so pod restarts during rollout
  # can still mount the Secret their Deployment references.
  lifecycle {
    create_before_destroy = true
  }
}

resource "kubernetes_manifest" "deployment" {
  field_manager {
    name            = local.field_manager
    force_conflicts = false
  }

  # spec.replicas is owned by HPA long-term and briefly by Flagger during
  # canary rollouts — excluded from drift detection here. The container
  # image is NOT in computed_fields: Flagger watches the Deployment for
  # spec changes but does not write back to it (it mutates its own
  # `-primary` sibling on promotion). So Terraform owns `image` normally;
  # `terraform apply` with a new var.image is what triggers a canary.
  computed_fields = [
    "metadata.annotations",
    "metadata.labels",
    "spec.replicas",
  ]

  manifest = {
    apiVersion = "apps/v1"
    kind       = "Deployment"
    metadata = {
      name      = var.name
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      selector = { matchLabels = local.selector_labels }
      template = {
        metadata = { labels = local.labels }
        spec = {
          serviceAccountName = var.name
          securityContext    = local.pod_security_context
          containers         = [local.container]
        }
      }
    }
  }

  # Explicit dependency on the Secret: the Deployment's envFrom references
  # local.secret_env_name, which is a static expression (not a resource
  # reference), so TF can't infer this ordering. Without it, the Deployment
  # update and Secret create/replace can run in either order, and pods
  # reaching the apiserver between steps see a stale or missing Secret.
  depends_on = [
    kubernetes_manifest.service_account,
    kubernetes_secret_v1.env,
  ]
}

resource "kubernetes_manifest" "service" {
  field_manager {
    name            = local.field_manager
    force_conflicts = true
  }

  manifest = {
    apiVersion = "v1"
    kind       = "Service"
    metadata = {
      name      = var.name
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      type     = "ClusterIP"
      selector = local.selector_labels
      ports = [{
        name       = "http"
        port       = var.port
        targetPort = var.port
      }]
    }
  }
}

resource "kubernetes_manifest" "hpa" {
  field_manager {
    name            = local.field_manager
    force_conflicts = true
  }

  manifest = {
    apiVersion = "autoscaling/v2"
    kind       = "HorizontalPodAutoscaler"
    metadata = {
      name      = var.name
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      minReplicas = var.autoscaling.min
      maxReplicas = var.autoscaling.max
      scaleTargetRef = {
        apiVersion = "apps/v1"
        kind       = "Deployment"
        name       = var.name
      }
      metrics = [{
        type = "Resource"
        resource = {
          name = "cpu"
          target = {
            type               = "Utilization"
            averageUtilization = var.autoscaling.target_cpu_utilization
          }
        }
      }]
    }
  }

  depends_on = [kubernetes_manifest.deployment]
}

# Only created when min_replicas >= 2: a PDB with min=max=1 is degenerate
# (blocks all voluntary disruption) and a PDB on a single replica is
# pointless (no alternative pod to preserve during drain).
resource "kubernetes_manifest" "pdb" {
  count = local.pdb_enabled ? 1 : 0

  field_manager {
    name            = local.field_manager
    force_conflicts = true
  }

  manifest = {
    apiVersion = "policy/v1"
    kind       = "PodDisruptionBudget"
    metadata = {
      name      = var.name
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      maxUnavailable = 1
      selector       = { matchLabels = local.selector_labels }
    }
  }
}

# VPA in InPlaceOrRecreate mode, scoped to memory on the "app" container:
# VPA actively resizes memory at runtime. CPU is deliberately excluded —
# the Google standard sets only requests.cpu on web services and VPA-tuning
# CPU would fight the "no CPU limit" shape. Requires the VPA CRD in the
# cluster; `terraform plan` fails otherwise. Disable via var.vpa_enabled.
resource "kubernetes_manifest" "vpa" {
  count = var.vpa_enabled ? 1 : 0

  field_manager {
    name            = local.field_manager
    force_conflicts = true
  }

  manifest = {
    apiVersion = "autoscaling.k8s.io/v1"
    kind       = "VerticalPodAutoscaler"
    metadata = {
      name      = var.name
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      targetRef = {
        apiVersion = "apps/v1"
        kind       = "Deployment"
        name       = var.name
      }
      updatePolicy = { updateMode = "InPlaceOrRecreate" }
      resourcePolicy = {
        containerPolicies = [{
          containerName       = local.container_name
          controlledResources = ["memory"]
        }]
      }
    }
  }

  depends_on = [kubernetes_manifest.deployment]
}
