locals {
  runtime_sa_id   = coalesce(var.service_account_id, "crj-${var.name}")
  scheduler_sa_id = "sch-${var.name}"
  memory_quantity = "${var.resources.memory}Mi"
}

resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = local.runtime_sa_id
  display_name = "Runtime identity for cronjob ${var.name}"
}

resource "google_service_account" "scheduler" {
  project      = var.project_id
  account_id   = local.scheduler_sa_id
  display_name = "Cloud Scheduler invoker for ${var.name}"
}

resource "google_project_iam_member" "scheduler_invoker" {
  project = var.project_id
  role    = "roles/run.invoker"
  member  = "serviceAccount:${google_service_account.scheduler.email}"
}

resource "google_cloud_run_v2_job" "this" {
  project  = var.project_id
  location = var.region
  name     = var.name

  template {
    parallelism = var.job.parallelism
    task_count  = var.job.completions

    template {
      service_account = google_service_account.runtime.email
      timeout         = "${var.job.timeout_seconds}s"
      max_retries     = var.job.max_retries

      containers {
        image = var.image
        args  = var.args

        resources {
          limits = {
            cpu    = tostring(var.resources.cpu)
            memory = local.memory_quantity
          }
        }

        dynamic "env" {
          for_each = var.env
          content {
            name  = env.key
            value = env.value
          }
        }

        dynamic "env" {
          for_each = var.secret_env
          content {
            name = env.key
            value_source {
              secret_key_ref {
                secret  = env.value.secret
                version = env.value.version
              }
            }
          }
        }
      }
    }
  }
}

resource "google_cloud_scheduler_job" "this" {
  project   = var.project_id
  region    = var.region
  name      = "${var.name}-trigger"
  schedule  = var.schedule.cron
  time_zone = var.schedule.time_zone

  attempt_deadline = try(var.cloud_scheduler.attempt_deadline_seconds, null) == null ? null : "${var.cloud_scheduler.attempt_deadline_seconds}s"

  dynamic "retry_config" {
    for_each = var.cloud_scheduler.retry_count > 0 ? [1] : []
    content {
      retry_count = var.cloud_scheduler.retry_count
    }
  }

  http_target {
    http_method = "POST"
    uri         = "https://${var.region}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${var.project_id}/jobs/${var.name}:run"

    oauth_token {
      service_account_email = google_service_account.scheduler.email
    }
  }
}
