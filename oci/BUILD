load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "oci",
    srcs = ["oci.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        "@grype.bzl//:grype",
        "@rules_img//img:image",
    ],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
    visibility = ["//visibility:public"],
)
