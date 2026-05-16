"Configuration for cc distroless images"

CC_DISTROS = ["debian", "hummingbird"]

CC_ARCHITECTURES = {
    # "debian12": ["amd64", "arm64", "arm", "s390x", "ppc64le"],
    "debian": ["amd64", "arm64"],
    "hummingbird": ["amd64", "arm64"],
}

def cc_layers(ctx):
    """Layer composition for cc images — no base inheritance.

    Composes static's content layer + cc's packages explicitly (plus busybox
    for the `_debug` variant) and one merged rpmdb on hummingbird. Dropping
    `base =` means we ship one rpmdb per image instead of N along the
    inheritance chain.

    Used for both release and debug variants — branches on `ctx.mode`.
    """
    layers = [
        "//oci/distroless/static:static_{}_{}_layer".format(ctx.arch, ctx.distro),
    ]
    if ctx.mode == "_debug":
        layers.append("//oci/distroless/static:busybox_{}_{}_layer".format(ctx.arch, ctx.distro))
    layers.append(":cc_{}_{}_layer".format(ctx.arch, ctx.distro))
    if ctx.distro == "hummingbird":
        rpmdb = ":rpmdb_cc_debug_{}_hummingbird" if ctx.mode == "_debug" else ":rpmdb_cc_{}_hummingbird"
        layers.append(rpmdb.format(ctx.arch))
    return layers
