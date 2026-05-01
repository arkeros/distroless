BASH_DISTROS = ["debian"]
BASH_ARCHITECTURES = {
    # "debian12": ["amd64", "arm64", "arm", "s390x", "ppc64le"],
    "debian": ["amd64", "arm64"],
}

def bash_layers(ctx):
    return [
        ":{}_{}_layer".format(ctx.arch, ctx.distro),
    ]
