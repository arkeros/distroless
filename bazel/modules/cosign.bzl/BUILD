load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

# Re-export shim: lets consumers `load("@cosign.bzl", "cosign_sign")` instead of
# the longer `@cosign.bzl//cosign:defs.bzl`. Same shape as `@tar.bzl`.
bzl_library(
    name = "cosign",
    srcs = ["cosign.bzl"],
    deps = [
        "//cosign:defs",
        "//cosign/toolchain",
    ],
)

exports_files(["cosign.bzl"])
