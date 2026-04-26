load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "gcp",
    srcs = ["gcp.bzl"],
    visibility = ["//visibility:public"],
    deps = ["//devtools/build/tools/tf:defs"],
)

bzl_library(
    name = "k8s",
    srcs = ["k8s.bzl"],
    visibility = ["//visibility:public"],
    deps = ["//devtools/build/tools/tf:defs"],
)
