load(
    "//devtools/bifrost/terraform/modules/service_kubernetes:defs.bzl",
    "service_kubernetes",
)
load("//devtools/build/tools/tf:defs.bzl", "tf_root")
load("//devtools/build/tools/tf/resources:gcp.bzl", "google_provider")

# Smoke target — never applied, never planned. Builds the generated tf.json
# so a regression in `service_kubernetes` (the Starlark macro) fails analysis
# instead of slipping into a real root.
PROJECT = "senku-prod"

REGION = "europe-west1"

svc = service_kubernetes(
    name = "api",
    autoscaling = {
        "max": 5,
        "min": 2,
    },
    env = {
        "DB_NAME": "api",
    },
    image = "europe-docker.pkg.dev/senku-prod/api/api@sha256:" + ("1" * 64),
    namespace = "default",
    probes = {
        "liveness_path": "/healthz",
        "readiness_path": "/ready",
        "startup_path": "/healthz",
    },
    project = PROJECT,
    resources = {
        "cpu": 0.25,
        "memory": 512,
    },
    secret_env = {
        "DB_PASSWORD": {
            "project": PROJECT,
            "secret": "api-db-password",
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
        svc,
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
