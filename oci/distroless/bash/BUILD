load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_img//img:push.bzl", "image_push")
load("//oci:config.bzl", "OCI_REGISTRY", "OCI_REPOSITORY_PREFIX")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE")
load(":config.bzl", "BASH_ARCHITECTURES", "BASH_DISTROS", "bash_layers")

package(default_visibility = ["//visibility:public"])

[
    distroless_matrix(
        name = "bash",
        architectures = BASH_ARCHITECTURES[distro],
        base = "//oci/distroless/cc",
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless bash image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/bash/BUILD",
        },
        distro = distro,
        entrypoint = ["/usr/bin/bash"],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless bash image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/bash/BUILD",
        },
        layers = bash_layers,
    )
    for distro in BASH_DISTROS
]

[
    flatten(
        name = arch + "_" + distro + "_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@{}//{}/{}".format(distro, "bash", arch),
            # for runfiles we need: env, sed, grep & awk
            # rust-coreutils provides /usr/bin/env without libsystemd0 dependency
            "@{}//{}/{}".format(distro, "rust-coreutils", arch),
            "//oci/distroless/common:coreutils_links",
            "@{}//{}/{}".format(distro, "sed", arch),
            "@{}//{}/{}".format(distro, "grep", arch),
            "@{}//{}/{}".format(distro, "mawk", arch),
            "//oci/distroless/common:awk",
        ],
    )
    for distro in BASH_DISTROS
    for arch in BASH_ARCHITECTURES[distro]
]

[
    image_push(
        name = "bash" + mode + "_push",
        image = ":bash" + mode + "_nonroot_debian13",
        registry = OCI_REGISTRY,
        repository = OCI_REPOSITORY_PREFIX + "/bash",
        stamp = "enabled",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
    )
    for mode in DEBUG_MODE
]

container_structure_test(
    name = "bash_test",
    size = "small",
    configs = ["test.yaml"],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":bash_nonroot_amd64_debian13.oci_layout",
    tags = ["requires-docker"],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
