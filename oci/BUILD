load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load(":cosign_policy.bzl", "BUILDER_ID", "CERTIFICATE_IDENTITY_REGEXP", "CERTIFICATE_OIDC_ISSUER")

bzl_library(
    name = "aspects",
    srcs = ["aspects.bzl"],
    visibility = ["//visibility:public"],
    deps = [":config"],
)

# Sourceable shell file mirroring `cosign_policy.bzl`'s consumer-side
# constants. CI's verify steps `cat` this into `$GITHUB_ENV` so the regex /
# issuer pinning come from the same source as the producer's BUILDER_ID.
write_file(
    name = "cosign_policy_env",
    out = "cosign_policy.env",
    content = [
        "COSIGN_POLICY_BUILDER_ID=" + BUILDER_ID,
        "COSIGN_POLICY_CERTIFICATE_IDENTITY_REGEXP=" + CERTIFICATE_IDENTITY_REGEXP,
        "COSIGN_POLICY_CERTIFICATE_OIDC_ISSUER=" + CERTIFICATE_OIDC_ISSUER,
        "",
    ],
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
    name = "mirror_push",
    srcs = ["mirror_push.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":config",
        ":cosign_policy",
        "@cosign.bzl//:cosign",
        "@rules_img//img:push",
    ],
)

bzl_library(
    name = "created_timestamp",
    srcs = ["created_timestamp.bzl"],
    visibility = ["//visibility:public"],
    deps = ["@jq.bzl//jq:jq"],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "cosign_policy",
    srcs = ["cosign_policy.bzl"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "vex",
    srcs = ["vex.bzl"],
    visibility = ["//visibility:public"],
)
