load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

# Exposed so //oci/distroless/common:created_debian can pin its snapshot
# anchor against the canonical apt lockfile.
exports_files(["debian.lock.json"])

bzl_library(
    name = "matrix",
    srcs = ["matrix.bzl"],
    visibility = ["//oci/distroless:__subpackages__"],
    deps = [
        "//oci:oci_image",
        "//oci/distroless:platforms",
        "//oci/distroless/common:variables",
        "@rules_img//img:image",
    ],
)

bzl_library(
    name = "platforms",
    srcs = ["platforms.bzl"],
    visibility = ["//visibility:public"],
)
