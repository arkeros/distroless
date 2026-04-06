resource "google_service_account" "crj_analytics_data_export" {
  project      = "senku-prod"
  account_id   = "crj-analytics-data-export"
  display_name = "Runtime identity for analytics-data-export"
}

resource "google_service_account_iam_member" "crj_analytics_data_export_workload_identity" {
  service_account_id = google_service_account.crj_analytics_data_export.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:senku-prod.svc.id.goog[jobs/analytics-data-export]"
}

resource "google_service_account" "sch_analytics_data_export" {
  project      = "senku-prod"
  account_id   = "sch-analytics-data-export"
  display_name = "Cloud Scheduler invoker for analytics-data-export"
}

resource "google_project_iam_member" "sch_analytics_data_export_run_invoker" {
  project = "senku-prod"
  role    = "roles/run.invoker"
  member  = format("serviceAccount:%s", google_service_account.sch_analytics_data_export.email)
}

resource "google_cloud_scheduler_job" "analytics_data_export_schedule" {
  project   = "senku-prod"
  name      = "analytics-data-export"
  region    = "europe-west1"
  schedule  = "0 12 * * *"
  time_zone = "Europe/Madrid"
  retry_config {
    retry_count = 3
  }
  http_target {
    http_method = "POST"
    uri         = "https://run.googleapis.com/v2/projects/senku-prod/locations/europe-west1/jobs/analytics-data-export:run"
    body        = base64encode("{}")
    headers = {
      Content-Type = "application/json"
    }
    oauth_token {
      service_account_email = google_service_account.sch_analytics_data_export.email
    }
  }
}
