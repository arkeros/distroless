load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "js_image",
    srcs = ["js_image.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":config",
        ":oci_image",
        "//oci/distroless:platforms",
        "//oci/distroless/common:variables",
        "@aspect_rules_js//js:defs",
        "@rules_img//img:image",
    ],
)

bzl_library(
    name = "py_image",
    srcs = ["py_image.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":config",
        ":oci_image",
        "//oci/distroless:platforms",
        "//oci/distroless/common:variables",
        "@aspect_rules_py//py:defs",
        "@rules_img//img:image",
    ],
)

bzl_library(
    name = "frontend_image",
    srcs = ["frontend_image.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":oci_image",
        "//oci/distroless:platforms",
        "//oci/distroless/common:variables",
        "//oci/distroless/nginx:config",
        "@rules_img//img:image",
    ],
)

bzl_library(
    name = "oci_image",
    srcs = ["oci_image.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":supply_chain",
        "@rules_img//img:image",
        "@rules_img//img:load",
        "@rules_img//img:push",
    ],
)

bzl_library(
    name = "supply_chain",
    srcs = ["supply_chain.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        "@grype.bzl//grype:defs",
        "@supply_chain_tools//sbom",
    ],
)

bzl_library(
    name = "vex",
    srcs = ["vex.bzl"],
    visibility = ["//visibility:public"],
)
