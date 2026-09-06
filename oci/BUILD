load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:copy_file.bzl", "copy_file")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@tar.bzl", "tar")
load(":cosign_policy.bzl", "CERTIFICATE_IDENTITY_REGEXP", "CERTIFICATE_OIDC_ISSUER", "PROVENANCE_BUILDER_ID", "PROVENANCE_SOURCE_URI")
load(":layer_paths.bzl", "layer_unique_paths_test")

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

# The Sigstore trusted root CI's `publish` and `verify` jobs hand to cosign via
# --trusted-root. The same pinned file //web embeds and verifies with, so the
# two can never disagree: the day Sigstore publishes a new root, the http_file
# checksum fails in CI, and the one bump that fixes CI fixes the Directory.
# Fetching the root live over TUF in CI — the previous design — would have kept
# CI green while the Directory went dark. See bazel/include/oci.MODULE.bazel.
#
# A copy rather than an alias so the file is a regular output under
# `bazel-out/`, which `bazel cquery --output=files` reports as a path that
# resolves from the workspace like the other tools CI picks up; the http_file's
# own path is execroot-relative (`external/...`) and does not.
copy_file(
    name = "sigstore_trusted_root",
    src = "@sigstore_trusted_root//file",
    out = "sigstore_trusted_root.json",
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

# The fixture for layer_unique_paths_test, the shape of the real defect:
# an rpm content tar, whose directories rpm-extract names without a
# trailing slash, beside a bsdtar-written tar naming one of the same
# directories with it. flatten keeps both, since it deduplicates the two
# strings, and dockerd refuses the layer. The tar alone must pass.
tar(
    name = "zoneinfo_dir",
    testonly = True,
    mtree = ["./usr/share/zoneinfo uid=0 gid=0 mode=0755 time=0.0 type=dir"],
)

flatten(
    name = "duplicate_paths_fixture",
    testonly = True,
    compress = "zstd",
    deduplicate = True,
    tars = [
        "@hummingbird//tzdata/noarch",
        ":zoneinfo_dir",
    ],
)

layer_unique_paths_test(
    name = "layer_unique_paths_finds_duplicates_test",
    size = "small",
    expect_duplicates = True,
    layers = [":duplicate_paths_fixture"],
)

layer_unique_paths_test(
    name = "layer_unique_paths_passes_unique_test",
    size = "small",
    layers = [
        "@hummingbird//tzdata/noarch",
        ":zoneinfo_dir",
    ],
)

bzl_library(
    name = "oci_image",
    srcs = ["oci_image.bzl"],
    visibility = ["//visibility:public"],
    deps = [
        ":layer_paths",
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
    name = "layer_paths",
    srcs = ["layer_paths.bzl"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "vex",
    srcs = ["vex.bzl"],
    visibility = ["//visibility:public"],
)
