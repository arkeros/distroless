terraform {
  required_version = ">= 1.14.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

provider "google" {
  project = "senku-prod"
  region  = "europe-west1"
}

module "daily_report" {
  source = "../.."

  name       = "daily-report"
  project_id = "senku-prod"
  region     = "europe-west1"

  image     = "europe-docker.pkg.dev/senku-prod/jobs/daily-report@sha256:0000000000000000000000000000000000000000000000000000000000000000"
  resources = { cpu = 1, memory = 512 }

  schedule = {
    cron      = "0 8 * * *"
    time_zone = "Europe/Madrid"
  }

  secret_env = {
    REPORT_API_KEY = {
      project = "senku-prod"
      secret  = "report-api-key"
      version = "2"
    }
  }

  cloud_scheduler = {
    retry_count              = 2
    attempt_deadline_seconds = 300
  }
}
