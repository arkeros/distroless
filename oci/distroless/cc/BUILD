load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_tar//tar/filter:defs.bzl", "tar_filter")
load("//oci:vex.bzl", "vex_document")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:glibc_vex.bzl", "GLIBC_STRIPPED_GCONV_VEX")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN13_WONTFIX_CVES", "GLIBC_STRIPPED_GCONV")
load(":config.bzl", "CC_ARCHITECTURES", "CC_DISTROS", "cc_layers")

package(default_visibility = ["//visibility:public"])

# Single layer for the whole cc tree. Closure refs (not `:data`) preserve
# `PackageMetadataInfo` for SBOM — alias targets don't propagate
# `applicable_licenses` through the gather_metadata aspect. Post-flatten
# `tar_filter` strips paths from the merged layer; doing it pre-flatten
# wouldn't help because the libstdc++6 closure transitively re-introduces
# unstripped libc6 paths and flatten's dedup can't *remove* paths only
# present in some inputs.
[
    flatten(
        name = "{}_{}_layer_raw".format(arch, distro),
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
    tar_filter(
        name = "{}_{}_layer.tar.zst".format(arch, distro),
        src = ":{}_{}_layer_raw".format(arch, distro),
        compress = "@rules_tar//tar/compress:zstd",
        patterns = GLIBC_STRIPPED_GCONV,
    )
    for distro in CC_DISTROS
    for arch in CC_ARCHITECTURES[distro]
]

vex_document(
    name = "vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/cc/BUILD",
    statements = [GLIBC_STRIPPED_GCONV_VEX],
    timestamp = "2026-05-01T00:00:00Z",
)

[
    distroless_matrix(
        name = "cc",
        architectures = CC_ARCHITECTURES[distro],
        base = "//oci/distroless/static",
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES,
        distro = distro,
        ignore_cves = DEBIAN13_WONTFIX_CVES,
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
    image = ":cc_nonroot_amd64_debian13.tar",
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
