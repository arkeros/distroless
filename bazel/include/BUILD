load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

# Make the terraform pin sources readable by the repin check test
# (which reads them via runfiles to assert the pin file is in sync
# with the MODULE.bazel tags).
exports_files(
    [
        "terraform.MODULE.bazel",
        "terraform.providers.lock.bzl",
    ],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "terraform.providers.lock",
    srcs = ["terraform.providers.lock.bzl"],
    visibility = ["//visibility:public"],
)
