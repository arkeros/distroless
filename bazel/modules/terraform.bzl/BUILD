load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

# Re-export shims: short consumer-facing load paths.
#
# - `load("@terraform.bzl", ...)`            — core defs + lint + toolchain.
# - `load("@terraform.bzl//:gcp.bzl", ...)`  — GCP resource constructors.
# - `load("@terraform.bzl//:k8s.bzl", ...)`  — Kubernetes resource constructors.
#
# Each consumer bzl_library deps on the specific shim(s) it loads from,
# matching the namespace split (resource constructors are provider-specific,
# core defs are not).
bzl_library(
    name = "terraform",
    srcs = ["terraform.bzl"],
    deps = [
        "//terraform:defs",
        "//terraform:lint",
        "//terraform/toolchain",
    ],
)

bzl_library(
    name = "gcp",
    srcs = ["gcp.bzl"],
    deps = ["//terraform/resources:gcp"],
)

bzl_library(
    name = "k8s",
    srcs = ["k8s.bzl"],
    deps = ["//terraform/resources:k8s"],
)

exports_files([
    "gcp.bzl",
    "k8s.bzl",
    "terraform.bzl",
])
