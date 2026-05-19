load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

bzl_library(
    name = "gcp",
    srcs = ["gcp.bzl"],
    deps = ["//terraform:defs"],
)

bzl_library(
    name = "k8s",
    srcs = ["k8s.bzl"],
    deps = ["//terraform:defs"],
)
