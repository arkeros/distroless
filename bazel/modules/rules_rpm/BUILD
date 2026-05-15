load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//visibility:public"])

# Re-export shim: lets consumers `load("@rules_rpm", "rpm_package", "rpmdb_merge")`
# instead of the longer `@rules_rpm//rpm:defs.bzl`. Same shape as @cosign.bzl.
bzl_library(
    name = "rules_rpm",
    srcs = ["rules_rpm.bzl"],
    deps = ["//rpm:defs"],
)

exports_files(["rules_rpm.bzl"])
