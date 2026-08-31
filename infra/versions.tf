terraform {
  required_version = ">= 1.14.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }

    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
  }
}
