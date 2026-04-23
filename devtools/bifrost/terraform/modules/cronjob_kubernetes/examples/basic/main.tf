terraform {
  required_version = ">= 1.14.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.38"
    }
  }
}

provider "google" {
  project = "senku-prod"
  region  = "europe-west1"
}

provider "kubernetes" {
  config_path = "~/.kube/config"
}

module "cleanup" {
  source = "../.."

  name       = "cleanup"
  project_id = "senku-prod"
  namespace  = "jobs"

  image     = "europe-docker.pkg.dev/senku-prod/jobs/cleanup@sha256:0000000000000000000000000000000000000000000000000000000000000000"
  resources = { cpu = 0.25, memory = 256 }

  schedule = {
    cron      = "0 2 * * *"
    time_zone = "Europe/Madrid"
  }

  job = {
    timeout_seconds = 1800
  }
}
