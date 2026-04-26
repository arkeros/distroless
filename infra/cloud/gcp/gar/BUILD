load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("//devtools/build/tools/tf:defs.bzl", "output", "tf_root")
load("//devtools/build/tools/tf/resources:gcp.bzl", "google_provider")
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
    deps = ["//devtools/build/tools/tf/resources:gcp"],
)
