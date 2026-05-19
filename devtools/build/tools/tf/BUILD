load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

# `render.bzl` loads from `@jq.bzl//jq:jq.bzl`, `@rules_img//...`, and
# `@terraform.bzl`; their bzl_library targets either don't exist or have
# private visibility, so depend on only the one library that does.
bzl_library(
    name = "render",
    srcs = ["render.bzl"],
    visibility = ["//visibility:public"],
    deps = ["@terraform.bzl//:terraform"],
)
