load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

# The linter aspects live in linters.bzl and oxlint.bzl; `bazel run
# //:gazelle` keeps their bzl_library targets below current.

bzl_library(
    name = "linters",
    srcs = ["linters.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":oxlint",
        "@aspect_rules_lint//lint:buildifier",
        "@aspect_rules_lint//lint:shellcheck",
    ],
)

bzl_library(
    name = "oxlint",
    srcs = ["oxlint.bzl"],
    visibility = ["//visibility:public"],
    deps = ["@bazel_skylib//rules:common_settings"],
)
