load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("//oci:vex.bzl", "vex_document")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES")
load(":config.bzl", "CC_ARCHITECTURES", "CC_DISTROS", "cc_layers")

package(default_visibility = ["//visibility:public"])

vex_document(
    name = "vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/cc/BUILD",
    statements = GLIBC_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

vex_document(
    name = "debug_vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/cc/BUILD#debug",
    statements = BUSYBOX_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

# Single layer for the whole cc tree. Closure refs (not `:data`) preserve
# `PackageMetadataInfo` for SBOM — alias targets don't propagate
# `applicable_licenses` through the gather_metadata aspect.
[
    flatten(
        name = "{}_{}_layer".format(arch, distro),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@{}//libc6/{}".format(distro, arch),
            "@{}//libssl3t64/{}".format(distro, arch),
            "@{}//libstdc++6/{}".format(distro, arch),
        ],
    )
    for distro in CC_DISTROS
    for arch in CC_ARCHITECTURES[distro]
]

[
    distroless_matrix(
        name = "cc",
        architectures = CC_ARCHITECTURES[distro],
        base = "//oci/distroless/static",
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES,
        debug_vex = [":debug_vex"],
        distro = distro,
        ignore_cves = DEBIAN_WONTFIX_CVES,
        layers = cc_layers,
        vex = [":vex"],
    )
    for distro in CC_DISTROS
]

container_structure_test(
    name = "cc_test",
    size = "small",
    configs = ["test.yaml"],
    driver = "tar",
    image = ":cc_nonroot_amd64_debian.tar",
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
