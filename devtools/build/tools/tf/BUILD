load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "defs",
    srcs = ["defs.bzl"],
    visibility = ["//visibility:public"],
    deps = [":render"],
)

bzl_library(
    name = "render",
    srcs = ["render.bzl"],
    visibility = ["//visibility:public"],
    deps = [],
)

exports_files(
    [
        "run.sh",
        "dag.sh",
    ],
    visibility = ["//visibility:public"],
)
