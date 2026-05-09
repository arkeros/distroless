load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

# Platforms for Linux targets (used by cosign_sign / cosign_attest network calls
# from sandboxed actions or transitions, parallel to grype.bzl's pattern).
platform(
    name = "linux_amd64",
    constraint_values = [
        "@platforms//os:linux",
        "@platforms//cpu:x86_64",
    ],
)

platform(
    name = "linux_arm64",
    constraint_values = [
        "@platforms//os:linux",
        "@platforms//cpu:aarch64",
    ],
)

exports_files([
    "defs.bzl",
])

toolchain_type(
    name = "toolchain",
)

bzl_library(
    name = "defs",
    srcs = ["defs.bzl"],
    deps = [
        "//cosign/private:attest",
        "//cosign/private:predicate",
        "//cosign/private:sign",
    ],
)

bzl_library(
    name = "extensions",
    srcs = ["extensions.bzl"],
    deps = [":versions"],
)

bzl_library(
    name = "versions",
    srcs = ["versions.bzl"],
)
