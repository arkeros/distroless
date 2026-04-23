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

module "hello" {
  source = "../.."

  name       = "hello"
  project_id = "senku-prod"
  region     = "europe-west1"

  image     = "gcr.io/google-samples/hello-app@sha256:3f87c2db2eab75bf8e5a3a48d6be1f73bb2a0c1e7e34e08b3e7b7e3b7e3b7e3b"
  resources = { cpu = 0.5, memory = 512 }

  scaling = {
    min = 0
    max = 3
  }

  public = true

  custom_domains = [
    "hello.senku.example.com",
  ]
}

output "hello_dns_records" {
  value = module.hello.custom_domain_dns_records
}
