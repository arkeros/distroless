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

module "hello" {
  source = "../.."

  name       = "hello"
  namespace  = "default"
  project_id = "senku-prod"
  image      = "gcr.io/google-samples/hello-app@sha256:3f87c2db2eab75bf8e5a3a48d6be1f73bb2a0c1e7e34e08b3e7b7e3b7e3b7e3b"
  resources  = { cpu = 0.25, memory = 512 }

  autoscaling = {
    min = 1
    max = 3
  }
}
