terraform {
  required_version = ">= 1.12.0"

  backend "gcs" {
    bucket = "senku-prod-terraform-state"
    prefix = "terraform/state"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}
