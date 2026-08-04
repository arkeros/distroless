load("@bazel_skylib//rules:build_test.bzl", "build_test")
load("@terraform.bzl//:gcp.bzl", "google_provider")
load("@terraform.bzl", "tf_root")
load("//devtools/bifrost/modules:cloudrun.bzl", "cronjob_cloudrun")

# Smoke target — never planned, never applied, just analyzed.
PROJECT = "senku-prod"

REGION = "europe-west1"

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
        cronjob_cloudrun(
            name = "daily_report",
            cloud_scheduler = {
                "attempt_deadline_seconds": 300,
                "retry_count": 2,
            },
            image = "europe-docker.pkg.dev/senku-prod/jobs/daily-report@sha256:" + ("0" * 64),
            job_name = "daily-report",
            project = PROJECT,
            region = REGION,
            resources = {
                "cpu": 1,
                "memory": 512,
            },
            schedule = {
                "cron": "0 8 * * *",
                "time_zone": "Europe/Madrid",
            },
            secret_env = {
                "REPORT_API_KEY": {
                    "project": PROJECT,
                    "secret": "report-api-key",
                    "version": "2",
                },
            },
        ),
    ],
    required_providers = {
        "google": {
            "source": "hashicorp/google",
            "version": "~> 7.0",
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
