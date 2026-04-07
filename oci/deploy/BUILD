load("@kustomize.bzl", "kustomize")
load("//devtools/bifrost:bifrost.bzl", "bifrost_service")

PROD = {
    "gcp": {
        "projectId": "senku-prod",
        "projectNumber": "874944788122",
        "region": "europe-west3",
    },
}

bifrost_service(
    name = "registry",
    args = [
        "--upstream=ghcr.io",
        "--repository-prefix=arkeros/senku",
    ],
    autoscaling = {
        "min": 0,
        "max": 3,
    },
    checked_in = {
        "terraform": "terraform/registry.generated.tf",
    },
    environment = PROD,
    image_push = "//oci/cmd/registry:image_push",
    probes = {
        "startupPath": "/v2/",
        "livenessPath": "/v2/",
    },
    resources = {
        "requests": {
            "cpu": "250m",
            "memory": "256Mi",
        },
        "limits": {
            "cpu": "1000m",
            "memory": "256Mi",
        },
    },
    targets = [
        "cloudrun",
        "terraform",
    ],
    visibility = ["//visibility:public"],
)

kustomize(
    name = "k8s",
    srcs = [
        "kustomization.yaml",
        ":registry_cloudrun",
    ],
    visibility = ["//visibility:public"],
)
