load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

# Re-export shim: lets consumers `load("@terraform.bzl", "tf_root")` instead
# of the longer `@terraform.bzl//terraform:defs.bzl`. Same shape as
# `@cosign.bzl` and `@tar.bzl`.
bzl_library(
    name = "terraform",
    srcs = ["terraform.bzl"],
    deps = [
        "//terraform:defs",
        "//terraform:lint",
        "//terraform/toolchain",
    ],
)

exports_files(["terraform.bzl"])
