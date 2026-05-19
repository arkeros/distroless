load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load(":hcl_test.bzl", "hcl_test_suite")

package(default_visibility = ["//visibility:public"])

# Direct unit tests for the HCL parser. Catches parser regressions
# without needing a full module-extension build cycle.
hcl_test_suite(name = "hcl_test")

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
    deps = [":platforms"],
)

bzl_library(
    name = "platforms",
    srcs = ["platforms.bzl"],
)

bzl_library(
    name = "extensions",
    srcs = ["extensions.bzl"],
    deps = [
        ":hcl",
        ":platforms",
        ":versions",
    ],
)

bzl_library(
    name = "hcl",
    srcs = ["hcl.bzl"],
)

bzl_library(
    name = "hcl_test_lib",
    srcs = ["hcl_test.bzl"],
    deps = [
        ":hcl",
        "@bazel_skylib//lib:unittest",
    ],
)

bzl_library(
    name = "versions",
    srcs = ["versions.bzl"],
)

exports_files([
    "defs.bzl",
    "lint_wrapper.sh.tpl",
    "pin.sh.tpl",
    "run.sh",
    "runner.sh.tpl",
])
