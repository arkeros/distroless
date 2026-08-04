load("@bazel_skylib//rules:build_test.bzl", "build_test")
load("@terraform.bzl//:gcp.bzl", "google_provider", "service_account")
load("@terraform.bzl", "tf_root")
load("//devtools/bifrost/modules:cloudrun.bzl", "service_cloudrun")

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

# The comment above says "just analyzed"; this is what makes that true rather
# than aspirational. `deploy = False` keeps the root out of every apply, and
# this keeps it building — so a regression in the macros it exercises fails
# here instead of rotting unnoticed.
build_test(
    name = "terraform_build_test",
    targets = [":terraform"],
)
