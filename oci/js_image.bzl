load("@aspect_rules_js//js:defs.bzl", "js_image_layer")
load("@rules_img//img:image.bzl", "image_index")
load("//oci/distroless:platforms.bzl", "ARCHITECTURE_PLATFORMS")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE", "USERS", "USER_IDS")
load(":oci_image.bzl", "oci_image")
load(":config.bzl", "NODEJS_ARCHITECTURES", "NODEJS_DISTROS")

def js_image_index(name, distro, architectures):
    """nodejs image index for a distro

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

def js_image(
        name,
        distro,
        arch,
        binary,
        entrypoint,
        extra_layers = [],
        ignore_cves = None):
    [
        js_image_layer(
            name = name + "_" + user + "_" + arch + "_layer",
            binary = binary,
            platform = ARCHITECTURE_PLATFORMS[arch],
            root = "/",
            compression = COMPRESSION,
            owner = "%s:%s" % (USER_IDS[user], USER_IDS[user]),
        )
        for user in USERS
    ]

    [
        oci_image(
            name = name + mode + "_" + user + "_" + arch,
            base = Label("//oci/distroless/bash:bash" + mode + "_" + user + "_" + arch + "_" + distro),
            entrypoint = entrypoint,
            working_dir = "/",
            env = {
                "NODE_ENV": "production",
            },
            layers = extra_layers + [
                name + "_" + user + "_" + arch + "_layer",
            ],
            platform = ARCHITECTURE_PLATFORMS[arch],
            ignore_cves = ignore_cves,
        )
        for mode in DEBUG_MODE
        for user in USERS
    ]

def js_images_all_arch(name, distro, binary, entrypoint, extra_layers = {}, ignore_cves = None):
    """nodejs images for all architectures for a distro

    Args:
        name: base name of image
        distro: name of distribution
        binary: binary target to include in image
        entrypoint: entrypoint for image
        extra_layers: dictionary of layers to include in the image, per architecture
    """
    architectures = NODEJS_ARCHITECTURES[distro]

    [
        js_image(
            name = name,
            distro = distro,
            arch = arch,
            binary = binary,
            entrypoint = entrypoint,
            extra_layers = extra_layers.get(arch, []),
            ignore_cves = ignore_cves,
        )
        for arch in architectures
    ]

    js_image_index(
        name = name,
        architectures = architectures,
        distro = distro,
    )
