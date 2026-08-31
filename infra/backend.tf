terraform {
  backend "gcs" {
    bucket = "senku-prod-terraform-state"
    prefix = "infra"
  }
}
