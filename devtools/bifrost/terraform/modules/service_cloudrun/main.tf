locals {
  create_service_account = var.service_account_email == null
  service_account_id     = coalesce(var.service_account_id, "svc-${var.name}")
  service_account_email  = local.create_service_account ? google_service_account.runtime[0].email : var.service_account_email
  labels                 = merge({ app = var.name }, var.labels)
  cpu_quantity           = tostring(var.resources.cpu)
  memory_quantity        = "${var.resources.memory}Mi"

  plain_env = [
    for k, v in var.env : {
      name  = k
      value = v
    }
  ]

  secret_env = [
    for k, v in var.secret_env : {
      name = k
      value_source = {
        secret_key_ref = {
          secret  = v.secret
          version = v.version
        }
      }
    }
  ]
}

resource "google_service_account" "runtime" {
  count = local.create_service_account ? 1 : 0

  project      = var.project_id
  account_id   = local.service_account_id
  display_name = "Runtime identity for ${var.name}"
}

resource "google_cloud_run_v2_service" "this" {
  project  = var.project_id
  location = var.region
  name     = var.name
  ingress  = var.ingress
  labels   = local.labels

  # Services in this repo are fully reproducible from code; the Google
  # default (`true`) blocks destroy-and-recreate when a service's `name`
  # changes or the workload moves regions. Opt out.
  deletion_protection = false

  template {
    service_account                  = local.service_account_email
    execution_environment            = var.execution_environment
    max_instance_request_concurrency = var.concurrency
    timeout                          = "${var.timeout_seconds}s"

    scaling {
      min_instance_count = var.scaling.min
      max_instance_count = var.scaling.max
    }

    containers {
      image = var.image
      args  = var.args

      ports {
        container_port = var.port
      }

      resources {
        limits = {
          cpu    = local.cpu_quantity
          memory = local.memory_quantity
        }
        cpu_idle          = var.cpu_idle
        startup_cpu_boost = var.startup_cpu_boost
      }

      dynamic "env" {
        for_each = local.plain_env
        content {
          name  = env.value.name
          value = env.value.value
        }
      }

      dynamic "env" {
        for_each = local.secret_env
        content {
          name = env.value.name
          value_source {
            secret_key_ref {
              secret  = env.value.value_source.secret_key_ref.secret
              version = env.value.value_source.secret_key_ref.version
            }
          }
        }
      }

      dynamic "startup_probe" {
        for_each = try(var.probes.startup_path, null) == null ? [] : [var.probes.startup_path]
        content {
          http_get {
            path = startup_probe.value
            port = var.port
          }
        }
      }

      dynamic "liveness_probe" {
        for_each = try(var.probes.liveness_path, null) == null ? [] : [var.probes.liveness_path]
        content {
          http_get {
            path = liveness_probe.value
            port = var.port
          }
        }
      }
    }
  }

  # Default traffic policy: latest revision takes 100%. Callers wanting
  # canary traffic splits override this at the resource level by setting
  # traffic blocks explicitly in a wrapper — not exposed as a module input
  # because it requires revision names that only exist post-apply.
  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }
}

# Grant public invoke access when var.public = true. Otherwise the service
# is callable only by identities holding roles/run.invoker via other means.
resource "google_cloud_run_v2_service_iam_member" "public" {
  count = var.public ? 1 : 0

  project  = var.project_id
  location = google_cloud_run_v2_service.this.location
  name     = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Cloud Run domain mappings. The v1 API resource is what the google provider
# exposes; it interoperates with v2 services by referencing the service name.
# The parent project must have the domain verified in Search Console.
resource "google_cloud_run_domain_mapping" "custom" {
  for_each = toset(var.custom_domains)

  name     = each.value
  location = var.region

  metadata {
    namespace = var.project_id
    labels    = local.labels
  }

  spec {
    route_name = google_cloud_run_v2_service.this.name
  }
}
