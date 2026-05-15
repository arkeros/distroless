BASH_DISTROS = ["debian", "hummingbird"]

BASH_ARCHITECTURES = {
    # "debian12": ["amd64", "arm64", "arm", "s390x", "ppc64le"],
    "debian": ["amd64", "arm64"],
    "hummingbird": ["amd64", "arm64"],
}

def bash_layers(ctx):
    layers = [":{}_{}_layer".format(ctx.arch, ctx.distro)]
    if ctx.distro == "hummingbird":
        # bash adds 5 packages on top of cc (bash, coreutils, sed, grep,
        # mawk). cc inherits static's rpmdb via its `base`; layering this
        # rpmdb on top overwrites the inherited one with the full
        # 13-package set (3 static + 5 cc + 5 bash).
        layers.append(":rpmdb_bash_{}_hummingbird".format(ctx.arch))
    return layers
