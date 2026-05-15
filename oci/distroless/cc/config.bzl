"Configuration for cc distroless images"

CC_DISTROS = ["debian", "hummingbird"]

CC_ARCHITECTURES = {
    # "debian12": ["amd64", "arm64", "arm", "s390x", "ppc64le"],
    "debian": ["amd64", "arm64"],
    "hummingbird": ["amd64", "arm64"],
}

def cc_layers(ctx):
    layers = [":cc_{}_{}_layer".format(ctx.arch, ctx.distro)]
    if ctx.distro == "hummingbird":
        # cc inherits static's rpmdb layer via `base = //oci/distroless/static`,
        # but adds 5 glibc-bearing packages. Overlay a fresh rpmdb at the same
        # path (/usr/lib/sysimage/rpm/rpmdb.sqlite) covering static's set + cc's
        # — otherwise syft would miss them and grype would fall through to NVD
        # CPE matching (the silent-zero trap from ADR 0007 §rpmdb sqlite).
        layers.append(":rpmdb_cc_{}_hummingbird".format(ctx.arch))
    return layers
