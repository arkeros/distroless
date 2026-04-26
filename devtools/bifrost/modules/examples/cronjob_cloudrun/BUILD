load("//devtools/bifrost/modules:cloudrun.bzl", "cronjob_cloudrun")
load("//devtools/build/tools/tf:defs.bzl", "tf_root")
load("//devtools/build/tools/tf/resources:gcp.bzl", "google_provider")

# Smoke target — never planned, never applied, just analyzed.
PROJECT = "senku-prod"

REGION = "europe-west1"

tf_root(
    name = "terraform",
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
