# Demonstrates composition: a Cloud SQL instance provides the hostname for the
# service's env, a Secret Manager secret holds the DB password, and a migration
# Job must complete before the Deployment rolls out.

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

resource "google_sql_database_instance" "api" {
  name             = "api-db"
  database_version = "POSTGRES_16"
  region           = "europe-west1"

  settings {
    tier = "db-f1-micro"
    ip_configuration {
      ipv4_enabled    = false
      private_network = "projects/senku-prod/global/networks/default"
    }
  }
  deletion_protection = true
}

resource "google_secret_manager_secret" "db_password" {
  secret_id = "api-db-password"
  replication {
    auto {}
  }
}

resource "kubernetes_job_v1" "migrate" {
  metadata {
    name      = "api-migrate"
    namespace = "default"
  }

  spec {
    backoff_limit = 0
    template {
      metadata {
        labels = { "app.kubernetes.io/name" = "api-migrate" }
      }
      spec {
        restart_policy = "Never"
        container {
          name  = "migrate"
          image = "europe-docker.pkg.dev/senku-prod/api/migrate@sha256:0000000000000000000000000000000000000000000000000000000000000000"
          env {
            name  = "DB_HOST"
            value = google_sql_database_instance.api.private_ip_address
          }
        }
      }
    }
  }

  wait_for_completion = true
}

module "api" {
  source = "../.."

  name       = "api"
  namespace  = "default"
  project_id = "senku-prod"
  image      = "europe-docker.pkg.dev/senku-prod/api/api@sha256:1111111111111111111111111111111111111111111111111111111111111111"
  port       = 8080
  resources  = { cpu = 0.25, memory = 512 }

  env = {
    DB_HOST = google_sql_database_instance.api.private_ip_address
    DB_NAME = "api"
  }

  secret_env = {
    DB_PASSWORD = {
      project = "senku-prod"
      secret  = google_secret_manager_secret.db_password.secret_id
      version = "3"
    }
  }

  autoscaling = {
    min = 1
    max = 5
  }

  probes = {
    startup_path   = "/healthz"
    liveness_path  = "/healthz"
    readiness_path = "/ready"
  }

  depends_on = [kubernetes_job_v1.migrate]
}

# Grant the service account access to the DB password secret.
resource "google_secret_manager_secret_iam_member" "api_db_password" {
  secret_id = google_secret_manager_secret.db_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${module.api.service_account_email}"
}
