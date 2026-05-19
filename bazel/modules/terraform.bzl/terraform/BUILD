load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

bzl_library(
    name = "defs",
    srcs = ["defs.bzl"],
    deps = [
        ":lockfile",
        ":rule",
    ],
)

bzl_library(
    name = "rule",
    srcs = ["rule.bzl"],
    deps = ["//terraform/toolchain"],
)

bzl_library(
    name = "lockfile",
    srcs = ["lockfile.bzl"],
    deps = [":provider"],
)

bzl_library(
    name = "lint",
    srcs = ["lint.bzl"],
)

bzl_library(
    name = "provider",
    srcs = ["provider.bzl"],
)

bzl_library(
    name = "extensions",
    srcs = ["extensions.bzl"],
    deps = [
        ":hcl",
        ":versions",
    ],
)

bzl_library(
    name = "hcl",
    srcs = ["hcl.bzl"],
)

bzl_library(
    name = "versions",
    srcs = ["versions.bzl"],
)

exports_files([
    "defs.bzl",
    "lint_wrapper.sh.tpl",
    "run.sh",
    "runner.sh.tpl",
])
