# The upstream bash release the images ship, verified against
# //images:debian.lock.json by `bash_lock_version_test` — a lockfile bump that
# moves bash turns that test red rather than silently mistagging an image.
#
# Upstream only: Debian's revision (`-3+b1` at the time of writing) is
# packaging churn that a rebuild bumps without changing bash, and `+` is not
# a legal character in an OCI tag anyway.
BASH_VERSION = "5.3"

# What `latest` also answers to. Two levels, as nginx publishes: the release
# for pinning to it, the major for floating across it.
BASH_TAGS = [BASH_VERSION.split(".")[0], BASH_VERSION]

BASH_DISTROS = ["debian", "hummingbird"]

BASH_ARCHITECTURES = {
    # "debian12": ["amd64", "arm64", "arm", "s390x", "ppc64le"],
    "debian": ["amd64", "arm64"],
    "hummingbird": ["amd64", "arm64"],
}

def bash_layers(ctx):
    """Composition: static + (busybox if debug) + cc + bash + one rpmdb."""
    layers = [
        "//images/static:static_{}_{}_layer".format(ctx.arch, ctx.distro),
    ]
    if ctx.mode == "_debug":
        layers.append("//images/static:busybox_{}_{}_layer".format(ctx.arch, ctx.distro))
    layers += [
        "//images/cc:cc_{}_{}_layer".format(ctx.arch, ctx.distro),
        ":{}_{}_layer".format(ctx.arch, ctx.distro),
    ]
    if ctx.distro == "hummingbird":
        rpmdb = ":rpmdb_bash_debug_{}_hummingbird" if ctx.mode == "_debug" else ":rpmdb_bash_{}_hummingbird"
        layers.append(rpmdb.format(ctx.arch))
    return layers
