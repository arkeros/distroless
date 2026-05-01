load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("//oci:vex.bzl", "vex_document")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless:platforms.bzl", BUSYBOX_ARCHITECTURES = "ALL_ARCHITECTURES")
load("//oci/distroless/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES")
load(":busybox.bzl", "busybox_layer")
load(":config.bzl", "STATIC_ARCHITECTURES", "STATIC_DISTROS", "static_debug_layers", "static_layers")

package(default_visibility = ["//visibility:public"])

vex_document(
    name = "vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/static/BUILD",
    statements = GLIBC_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

vex_document(
    name = "debug_vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/static/BUILD#debug",
    statements = BUSYBOX_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

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
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES,
        debug_layers = static_debug_layers,
        debug_vex = [":debug_vex"],
        distro = distro,
        env = {
            "PATH": _DEFAULT_PATH,
            "SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
        },
        ignore_cves = DEBIAN_WONTFIX_CVES,
        layers = static_layers,
        vex = [":vex"],
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
        image = ":static_" + user + "_amd64_debian.tar",
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
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":static_debug_nonroot_amd64_debian.oci_layout",
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
