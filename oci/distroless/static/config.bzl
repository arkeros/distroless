"Configuration for static distroless images"

STATIC_DISTROS = ["debian"]

STATIC_ARCHITECTURES = {
    "debian": ["amd64", "arm64"],
}

def static_layers(ctx):
    return [
        ":static_{}_{}_layer".format(ctx.arch, ctx.distro),
    ]

def static_debug_layers(ctx):
    return [
        ":busybox_{}_{}_layer".format(ctx.arch, ctx.distro),
    ]
