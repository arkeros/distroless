terraform {
  # Loose floor that matches what the bazel module's `tf_root` itself emits
  # into each per-root `backend.tf.json`. Gates ad-hoc human invocations of
  # `terraform` against this file with a CLI older than what we generate
  # for. Intentionally NOT pinned to the bazel toolchain's exact `1.14.8`:
  # the lockfile content is independent of the terraform CLI version, so
  # this constraint only protects ad-hoc human runs, and a tighter pin
  # would add a fifth place we'd have to bump in lockstep.
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
