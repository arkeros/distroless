load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

# Loaded by the terraform module extension; needs to be a bzl_library so
# buildifier and the IDE see it, but the extension only needs the
# `load()` to resolve. Name matches gazelle's auto-generated convention
# (file basename without `.bzl`).
bzl_library(
    name = "terraform.providers.lock",
    srcs = ["terraform.providers.lock.bzl"],
    visibility = ["//visibility:public"],
)
