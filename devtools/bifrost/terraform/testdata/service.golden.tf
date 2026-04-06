resource "google_service_account" "svc_registry" {
  project      = "senku-prod"
  account_id   = "svc-registry"
  display_name = "Runtime identity for registry"
}

resource "google_service_account_iam_member" "svc_registry_workload_identity" {
  service_account_id = google_service_account.svc_registry.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:senku-prod.svc.id.goog[default/registry]"
}
