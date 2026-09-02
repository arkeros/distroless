load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

bzl_library(
    name = "sign",
    srcs = ["sign.bzl"],
    deps = ["//cosign/toolchain"],
)

bzl_library(
    name = "attest",
    srcs = ["attest.bzl"],
    deps = ["//cosign/toolchain"],
)

exports_files([
    "sign.sh.tpl",
    "attest.sh.tpl",
])
