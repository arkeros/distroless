load("@aspect_rules_py//py:defs.bzl", "py_image_layer")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_img//img:image.bzl", "image_index")
load("//oci/distroless:platforms.bzl", "ARCHITECTURE_PLATFORMS")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE", "USERS", "USER_IDS")
load(":oci_image.bzl", "oci_image")
load(":config.bzl", "PYTHON_ARCHITECTURES", "PYTHON_DISTROS")

def py_image_index(name, distro, architectures):
    """python image index for a distro

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

def py_image(
        name,
        distro,
        arch,
        binary):
    binary_label = native.package_relative_label(binary)
    binary_name = binary_label.name
    binary_path = binary_label.package

    [
        py_image_layer(
            name = name + "_" + user + "_" + arch + "_layer",
            binary = binary,
            compress = COMPRESSION,
            platform = ARCHITECTURE_PLATFORMS[arch],
            owner = str(USER_IDS[user]),
            group = str(USER_IDS[user]),
        )
        for user in USERS
    ]

    [
        oci_image(
            name = name + mode + "_" + user + "_" + arch,
            base = "//oci/distroless/bash:bash" + mode + "_" + user + "_" + arch + "_" + distro,
            entrypoint = ["/{}/{}".format(binary_path, binary_name)],
            # Use UTF-8 encoding for file system: match modern Linux
            env = {"LANG": "C.UTF-8"},
            layers = [
                "//distroless/python:" + arch + "_" + distro + "_layer",
                name + "_" + user + "_" + arch + "_layer",
            ],
            platform = ARCHITECTURE_PLATFORMS[arch],
        )
        for mode in DEBUG_MODE
        for user in USERS
    ]

def py_images_all_arch(name, distro, binary):
    """python images for all architectures for a distro

    Args:
        name: base name of image
        distro: name of distribution
        binary: binary target to include in image
    """
    architectures = PYTHON_ARCHITECTURES[distro]

    [
        py_image(
            name = name,
            distro = distro,
            arch = arch,
            binary = binary,
        )
        for arch in architectures
    ]

    py_image_index(
        name = name,
        architectures = architectures,
        distro = distro,
    )
