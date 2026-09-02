load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load(":cosign_policy.bzl", "CERTIFICATE_IDENTITY_REGEXP", "CERTIFICATE_OIDC_ISSUER", "PROVENANCE_BUILDER_ID", "PROVENANCE_SOURCE_URI")

bzl_library(
    name = "aspects",
    srcs = ["aspects.bzl"],
    visibility = ["//visibility:public"],
    deps = [":config"],
)

# Sourceable shell file mirroring `cosign_policy.bzl`'s consumer-side
# constants. CI's verify steps `cat` this into `$GITHUB_ENV` so the identity
# pins for both signers come from the one place the docs also derive from.
write_file(
    name = "cosign_policy_env",
    out = "cosign_policy.env",
    content = [
        "COSIGN_POLICY_CERTIFICATE_IDENTITY_REGEXP=" + CERTIFICATE_IDENTITY_REGEXP,
        "COSIGN_POLICY_CERTIFICATE_OIDC_ISSUER=" + CERTIFICATE_OIDC_ISSUER,
        "COSIGN_POLICY_PROVENANCE_BUILDER_ID=" + PROVENANCE_BUILDER_ID,
        "COSIGN_POLICY_PROVENANCE_SOURCE_URI=" + PROVENANCE_SOURCE_URI,
        "",
    ],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "frontend_image",
    srcs = ["frontend_image.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":oci_image",
        "//images:platforms",
        "//images/common:variables",
        "//images/nginx:config",
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
        ":crane_tag",
        "@cosign.bzl//:cosign",
        "@rules_img//img:push",
    ],
)

exports_files(["crane_tag.sh.tpl"])

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
    name = "crane_tag",
    srcs = ["crane_tag.bzl"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "vex",
    srcs = ["vex.bzl"],
    visibility = ["//visibility:public"],
)
