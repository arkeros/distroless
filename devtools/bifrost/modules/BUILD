load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "cloudrun",
    srcs = ["cloudrun.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        "//devtools/build/tools/tf:defs",
        "//devtools/build/tools/tf/resources:gcp",
    ],
)

bzl_library(
    name = "kubernetes",
    srcs = ["kubernetes.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        "//devtools/build/tools/tf:defs",
        "//devtools/build/tools/tf/resources:gcp",
        "//devtools/build/tools/tf/resources:k8s",
    ],
)
