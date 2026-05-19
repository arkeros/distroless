load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "cloudrun",
    srcs = ["cloudrun.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        "@terraform.bzl//:gcp",
        "@terraform.bzl//:terraform",
    ],
)

bzl_library(
    name = "kubernetes",
    srcs = ["kubernetes.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        "@terraform.bzl//:gcp",
        "@terraform.bzl//:k8s",
        "@terraform.bzl//:terraform",
    ],
)
