load("@tar.bzl", "tar")
load("@bazel_lib//lib:transitions.bzl", "platform_transition_filegroup")
load("@rules_img//img:image.bzl", "image_index")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE", "USERS")
load(":oci_image.bzl", "oci_image")
load(":config.bzl", "GO_ARCHITECTURES", "GO_DISTROS")

ARCHITECTURE_PLATFORMS = {
    "amd64": "@rules_go//go/toolchain:linux_amd64",
    "arm64": "@rules_go//go/toolchain:linux_arm64",
}

def go_image_index(name, distro, architectures):
    """go image index for a distro

    Args:
        name: base name of image
        distro: name of distribution
        architectures: all architectures included in index
    """
    for mode in DEBUG_MODE:
        for user in USERS:
            index_name = name + mode + "_" + user
            image_index(
                name = index_name,
                manifests = [
                    name + mode + "_" + user + "_" + arch
                    for arch in architectures
                ],
            )

def go_image(
        name,
        distro,
        arch,
        binary,
        entrypoint = None,
        cmd = None,
        extra_layers = [],
        static = True,
        registry = None,
        repository_prefix = None):
    """go image for a specific architecture

    Args:
        name: base name of image
        distro: name of distribution
        arch: architecture
        binary: binary target to include in image
        entrypoint: optional entrypoint override
        cmd: optional default command args
        extra_layers: additional layers to include
        static: if True, use static distroless; if False, use cc distroless with libstdc++6
        registry: container registry
        repository_prefix: repository prefix
    """
    binary_label = native.package_relative_label(binary)
    binary_name = binary_label.name
    binary_path = binary_label.package

    platform_transition_filegroup(
        name = name + "_" + arch + "_layer",
        srcs = [":" + name + "_layer"],
        target_platform = ARCHITECTURE_PLATFORMS[arch],
    )

    base_prefix = "//oci/distroless/static:static" if static else "//oci/distroless/cc:cc"

    # Build oci_image kwargs
    oci_kwargs = {
        "working_dir": "/",
        "platform": ARCHITECTURE_PLATFORMS[arch],
    }
    if registry != None:
        oci_kwargs["registry"] = registry
    if repository_prefix != None:
        oci_kwargs["repository_prefix"] = repository_prefix
    if cmd != None:
        oci_kwargs["cmd"] = cmd

    [
        oci_image(
            name = name + mode + "_" + user + "_" + arch,
            base = Label(base_prefix + mode + "_" + user + "_" + arch + "_" + distro),
            entrypoint = entrypoint if entrypoint != None else ["/{}/{}_/{}".format(binary_path, binary_name, binary_name)],
            layers = extra_layers + [
                name + "_" + arch + "_layer",
            ],
            **oci_kwargs
        )
        for mode in DEBUG_MODE
        for user in USERS
    ]

def go_images_all_arch(
        name,
        distro,
        binary,
        entrypoint = None,
        cmd = None,
        extra_layers = {},
        static = True,
        registry = None,
        repository_prefix = None):
    """go images for all architectures for a distro

    Args:
        name: base name of image
        distro: name of distribution
        binary: binary target to include in image
        entrypoint: optional entrypoint override
        cmd: optional default command args
        extra_layers: dictionary of layers to include in the image, per architecture
        static: if True, use static distroless; if False, use cc distroless with libstdc++6
        registry: container registry
        repository_prefix: repository prefix
    """
    architectures = GO_ARCHITECTURES[distro]

    tar(
        name = name + "_layer",
        srcs = [binary],
        compress = COMPRESSION,
    )

    [
        go_image(
            name = name,
            distro = distro,
            arch = arch,
            binary = binary,
            entrypoint = entrypoint,
            cmd = cmd,
            extra_layers = extra_layers.get(arch, []),
            static = static,
            registry = registry,
            repository_prefix = repository_prefix,
        )
        for arch in architectures
    ]

    go_image_index(
        name = name,
        architectures = architectures,
        distro = distro,
    )
