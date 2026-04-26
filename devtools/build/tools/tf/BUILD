load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "defs",
    srcs = ["defs.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":render",
        ":rule",
    ],
)

bzl_library(
    name = "render",
    srcs = ["render.bzl"],
    visibility = ["//visibility:public"],
    deps = [],
)

bzl_library(
    name = "rule",
    srcs = ["rule.bzl"],
    visibility = ["//visibility:public"],
    deps = ["//devtools/build/tools/tf/toolchain"],
)

exports_files(
    [
        "lint_wrapper.sh.tpl",
        "run.sh",
        "runner.sh.tpl",
    ],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "lockfile",
    srcs = ["lockfile.bzl"],
    visibility = ["//visibility:public"],
    deps = [":provider"],
)

bzl_library(
    name = "lint",
    srcs = ["lint.bzl"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "provider",
    srcs = ["provider.bzl"],
    visibility = ["//visibility:public"],
)
