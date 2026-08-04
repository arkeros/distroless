load("@bazel_skylib//rules:build_test.bzl", "build_test")
load("@terraform.bzl//:gcp.bzl", "google_provider")
load("@terraform.bzl", "tf_root")
load("//devtools/bifrost/modules:kubernetes.bzl", "cronjob_kubernetes")

# Smoke target — never planned, never applied, just analyzed.
PROJECT = "senku-prod"

REGION = "europe-west1"

NAMESPACE = "default"

tf_root(
    name = "terraform",
    # Two independent reasons this can never reach prod. `deploy = False`
    # keeps it out of the set `aspect apply` walks; the bucket name is
    # syntactically invalid for GCS (uppercase), so even a hand-run
    # `bazel run :terraform.apply` dies in `terraform init` — before auth,
    # before any provider call, before any resource exists.
    backend_bucket = "EXAMPLE-NEVER-APPLIED",
    deploy = False,
    docs = [
        google_provider(
            project = PROJECT,
            region = REGION,
        ),
        cronjob_kubernetes(
            name = "cleanup",
            image = "europe-docker.pkg.dev/senku-prod/jobs/cleanup@sha256:" + ("0" * 64),
            job = {"timeout_seconds": 1800},
            namespace = NAMESPACE,
            project = PROJECT,
            resources = {
                "cpu": 0.25,
                "memory": 256,
            },
            schedule = {
                "cron": "0 2 * * *",
                "time_zone": "Europe/Madrid",
            },
            secret_env = {
                "API_TOKEN": {
                    "project": PROJECT,
                    "secret": "cleanup-api-token",
                    "version": "3",
                },
            },
        ),
    ],
    required_providers = {
        "google": {
            "source": "hashicorp/google",
            "version": "~> 7.0",
        },
        "kubernetes": {
            "source": "hashicorp/kubernetes",
            "version": "~> 2.38",
        },
    },
    visibility = ["//visibility:private"],
)

# The comment above says "just analyzed"; this is what makes that true rather
# than aspirational. `deploy = False` keeps the root out of every apply, and
# this keeps it building — so a regression in the macros it exercises fails
# here instead of rotting unnoticed.
build_test(
    name = "terraform_build_test",
    targets = [":terraform"],
)
