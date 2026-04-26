load("//devtools/bifrost/modules:cloudrun.bzl", "service_cloudrun")
load("//devtools/build/tools/tf:defs.bzl", "tf_root")
load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "google_provider",
    "service_account",
)

# Smoke target — never planned, never applied, just analyzed. A regression
# in `service_cloudrun` (or the constructors it composes) fails Bazel
# analysis instead of slipping into a real root.

PROJECT = "senku-prod"

REGION = "europe-west1"

sa = service_account(
    name = "hello",
    account_id = "svc-hello",
    display_name = "Runtime identity for hello",
    project = PROJECT,
)

tf_root(
    name = "terraform",
    docs = [
        google_provider(
            project = PROJECT,
            region = REGION,
        ),
        sa,
        service_cloudrun(
            name = "hello",
            image = "europe-docker.pkg.dev/senku-prod/api/hello@sha256:" + ("1" * 64),
            ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER",
            project = PROJECT,
            public = True,
            region = REGION,
            resources = {
                "cpu": 1,
                "memory": 512,
            },
            scaling = {
                "max": 3,
                "min": 0,
            },
            service_account_email = sa.email,
            service_name = "hello",
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
