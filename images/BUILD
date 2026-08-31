load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

# The two distro package sets. debian.lock.json is exposed so
# //images/common:created_debian can pin its snapshot anchor against the
# canonical apt lockfile; the hummingbird pair is consumed by rules_rpm via
# @hummingbird//... — see bazel/include/oci.MODULE.bazel.
#
# Hummingbird has no manifest beside its lockfile the way debian.yaml sits
# beside debian.lock.json: its package list is declared inline in
# MODULE.bazel, per ADR 0007.
exports_files([
    "debian.lock.json",
    "hummingbird-release.pgp",
    "hummingbird.lock.json",
])

bzl_library(
    name = "matrix",
    srcs = ["matrix.bzl"],
    visibility = ["//images:__subpackages__"],
    deps = [
        ":platforms",
        "//images/common:variables",
        "//oci:oci_image",
        "@rules_img//img:image",
    ],
)

bzl_library(
    name = "platforms",
    srcs = ["platforms.bzl"],
    visibility = ["//visibility:public"],
)
