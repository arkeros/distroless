resource "google_service_account" "svc_registry" {
  project      = "senku-prod"
  account_id   = "svc-registry"
  display_name = "Runtime identity for registry"
}
