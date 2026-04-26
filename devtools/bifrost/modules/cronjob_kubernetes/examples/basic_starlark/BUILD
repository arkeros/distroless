load(
    "//devtools/bifrost/terraform/modules/cronjob_kubernetes:defs.bzl",
    "cronjob_kubernetes",
)
load("//devtools/build/tools/tf:defs.bzl", "tf_root")
load("//devtools/build/tools/tf/resources:gcp.bzl", "google_provider")

# Smoke target — never applied, never planned. Builds the generated tf.json
# so a regression in `cronjob_kubernetes` (the Starlark macro) fails analysis
# instead of slipping into a real root.
PROJECT = "senku-prod"

REGION = "europe-west1"

cronjob = cronjob_kubernetes(
    name = "cleanup",
    image = "europe-docker.pkg.dev/senku-prod/jobs/cleanup@sha256:" + ("0" * 64),
    job = {
        "timeout_seconds": 1800,
    },
    namespace = "jobs",
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
)

tf_root(
    name = "terraform",
    docs = [
        google_provider(
            project = PROJECT,
            region = REGION,
        ),
        cronjob,
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
