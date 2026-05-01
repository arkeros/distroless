CC_DISTROS = ["debian13"]
CC_ARCHITECTURES = {
    # "debian12": ["amd64", "arm64", "arm", "s390x", "ppc64le"],
    "debian13": ["amd64", "arm64"],
}

def cc_layers(ctx):
    return [
        ":{}_{}_layer.tar.zst".format(ctx.arch, ctx.distro),
    ]
