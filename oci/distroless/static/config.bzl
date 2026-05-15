"Configuration for static distroless images"

STATIC_DISTROS = ["debian", "hummingbird"]

# Hummingbird's static package set is all-noarch (tzdata, ca-certificates,
# mailcap), so amd64 and arm64 images share identical content layers — only
# the OCI manifest's platform field differs. arch-specific arches (e.g.
# glibc.x86_64 vs glibc.aarch64) land once cc migrates.
STATIC_ARCHITECTURES = {
    "debian": ["amd64", "arm64"],
    "hummingbird": ["amd64", "arm64"],
}

def static_layers(ctx):
    layers = [":static_{}_{}_layer".format(ctx.arch, ctx.distro)]
    if ctx.distro == "hummingbird":
        # rpmdb sqlite tar is a separate layer so syft's rpm-db cataloger
        # finds /usr/lib/sysimage/rpm/rpmdb.sqlite. Aspect-collected per
        # ADR 0007 §"aspect-driven rpmdb_merge".
        layers.append(":rpmdb_{}_hummingbird".format(ctx.arch))
    return layers

def static_debug_layers(ctx):
    layers = [":busybox_{}_{}_layer".format(ctx.arch, ctx.distro)]
    if ctx.distro == "hummingbird":
        # Overrides the release rpmdb (same /usr/lib/sysimage/rpm path) with
        # one that includes busybox — so grype routes its CVEs via the
        # hummingbird provider rather than NVD-generic-via-binary-cataloger.
        layers.append(":rpmdb_debug_{}_hummingbird".format(ctx.arch))
    return layers
