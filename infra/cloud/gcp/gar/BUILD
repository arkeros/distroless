load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@terraform.bzl//:gcp.bzl", "google_provider")
load("@terraform.bzl", "output", "tf_root")
load(
    ":defs.bzl",
    "ARTIFACT_REGISTRY_API",
    "CONTAINERS_REPO",
    "GAR_PROJECT",
    "GAR_REGISTRY",
    "GAR_REPOSITORY_PREFIX",
)

tf_root(
    name = "terraform",
    backend_bucket = "senku-prod-terraform-state",
    docs = [
        google_provider(project = GAR_PROJECT),
        ARTIFACT_REGISTRY_API,
        CONTAINERS_REPO,
        output(
            "repository_url",
            description = "Pull/push host + repo path. Prepend to an image name for a full reference, e.g. `<repository_url>/registry@sha256:<digest>`.",
            value = GAR_REGISTRY + "/" + GAR_REPOSITORY_PREFIX,
        ),
    ],
    providers = ["@terraform_providers//:google"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "defs",
    srcs = ["defs.bzl"],
    visibility = ["//visibility:public"],
    deps = ["@terraform.bzl//:gcp"],
)
