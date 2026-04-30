load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN13_WONTFIX_CVES")
load(":config.bzl", "CC_ARCHITECTURES", "CC_DISTROS", "CC_PACKAGES", "cc_layers")

package(default_visibility = ["//visibility:public"])

[
    flatten(
        name = arch + "_" + distro + "_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@{}//{}/{}".format(distro, pkg, arch)
            for pkg in CC_PACKAGES[distro]
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
        distro = distro,
        ignore_cves = DEBIAN13_WONTFIX_CVES,
        layers = cc_layers,
    )
    for distro in CC_DISTROS
]

container_structure_test(
    name = "cc_test",
    size = "small",
    configs = ["test.yaml"],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":cc_nonroot_amd64_debian13.oci_layout",
    tags = ["requires-docker"],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
