load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless:platforms.bzl", BUSYBOX_ARCHITECTURES = "ALL_ARCHITECTURES")
load("//oci/distroless/common:variables.bzl", "COMPRESSION")
load(":busybox.bzl", "busybox_layer")
load(":config.bzl", "STATIC_ARCHITECTURES", "STATIC_DISTROS", "static_debug_layers", "static_layers")

package(default_visibility = ["//visibility:public"])

_DEFAULT_PATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

[
    flatten(
        name = "static_" + arch + "_" + distro + "_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@{}//{}/{}".format(distro, "base-files", arch),
            "@{}//{}/{}".format(distro, "netbase", arch),
            "@{}//{}/{}".format(distro, "tzdata", arch),
            "@{}//{}/{}".format(distro, "media-types", arch),
            "//oci/distroless/common:rootfs",
            "//oci/distroless/common:passwd",
            "//oci/distroless/common:home",
            "//oci/distroless/common:group",
            "//oci/distroless/common:tmp",
            "//oci/distroless/common:os_release_" + distro,
            "//oci/distroless/common:cacerts_" + distro + "_" + arch,
        ],
    )
    for distro in STATIC_DISTROS
    for arch in STATIC_ARCHITECTURES[distro]
]

[
    distroless_matrix(
        name = "static",
        architectures = STATIC_ARCHITECTURES[distro],
        debug_entrypoint = ["/busybox/sh"],
        debug_env = {"PATH": _DEFAULT_PATH + ":/busybox"},
        debug_layers = static_debug_layers,
        distro = distro,
        env = {
            "PATH": _DEFAULT_PATH,
            "SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
        },
        layers = static_layers,
    )
    for distro in STATIC_DISTROS
]

[
    busybox_layer(
        name = "busybox_" + arch + "_" + distro + "_layer",
        busybox = "@{}//{}/{}".format(distro, "busybox-static", arch),
        compress = "gzip",
    )
    for arch in BUSYBOX_ARCHITECTURES
    for distro in STATIC_DISTROS
]

# Structure tests (amd64 only, tar driver doesn't need docker)
[
    container_structure_test(
        name = "static_" + user + "_test",
        size = "small",
        configs = [
            "test.yaml",
            "test_" + user + ".yaml",
        ],
        driver = "tar",
        image = ":static_" + user + "_amd64_debian13.tar",
    )
    for user in [
        "root",
        "nonroot",
    ]
]

container_structure_test(
    name = "static_debug_test",
    size = "small",
    configs = [
        "test.yaml",
        "test_debug.yaml",
    ],
    driver = "docker",
    image = ":static_debug_nonroot_amd64_debian13.oci_layout",
    tags = ["requires-docker"],
)

bzl_library(
    name = "busybox",
    srcs = ["busybox.bzl"],
    deps = [
        "//oci/distroless/common:variables",
        "@rules_distroless//distroless:defs",
    ],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
