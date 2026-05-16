NGINX_VERSIONS = {
    "stable": "nginx_stable",
    "mainline": "nginx_mainline",
}

# version_label -> short_version tag. Bumped 1.29 -> 1.31 when the
# nginx_mainline lockfile rolled to nginx 1.31.0-1~trixie (the
# `nginx_mainline_lock_version_test` jq_test enforces this stays in
# sync with the actual pinned version).
NGINX_TAGS = {
    "stable": "1.30",
    "mainline": "1.31",
}

def _version_parts(version):
    return tuple([int(part) for part in version.split(".")])

def _argmax_channel(tags):
    best_channel = None
    best_version = None

    for channel, version in tags.items():
        version_parts = _version_parts(version)
        if best_version == None or version_parts > best_version:
            best_channel = channel
            best_version = version_parts

    return best_channel

# `latest` intentionally tracks the highest supported nginx stream.
NGINX_LATEST_CHANNEL = _argmax_channel(NGINX_TAGS)

NGINX_DISTROS = ["debian"]

# Hummingbird-based images source nginx from nginx.org's own RPM repos
# (//bazel/include:oci.MODULE.bazel @nginx_stable_rpm / @nginx_mainline_rpm)
# — same posture as the apt side, which sources from pkg.nginx.org rather
# than Debian's nginx package. nginx.org publishes stable and mainline as
# first-class separate repos, so both channels are available without any
# version-constraint pinning logic.
NGINX_CHANNEL_DISTROS = {
    "stable": ["debian", "hummingbird"],
    "mainline": ["debian", "hummingbird"],
}

NGINX_ARCHITECTURES = {
    "debian": ["amd64", "arm64"],
    "hummingbird": ["amd64", "arm64"],
}

def nginx_layers(version_label):
    """Composition: static + (busybox if debug) + cc + nginx + conf + one rpmdb.

    No base inheritance — explicit layer list. Branches on ctx.mode for
    busybox in debug variants.

    Args:
        version_label: "stable" or "mainline"
    """

    def _layers(ctx):
        layers = [
            "//oci/distroless/static:static_{}_{}_layer".format(ctx.arch, ctx.distro),
        ]
        if ctx.mode == "_debug":
            layers.append("//oci/distroless/static:busybox_{}_{}_layer".format(ctx.arch, ctx.distro))
        layers += [
            "//oci/distroless/cc:cc_{}_{}_layer".format(ctx.arch, ctx.distro),
            ":{}_{}_{}_layer".format(version_label, ctx.arch, ctx.distro),
            ":nginx_conf_layer",
        ]
        if ctx.distro == "hummingbird":
            if ctx.mode == "_debug":
                layers.append(":rpmdb_nginx_debug_{}_{}_hummingbird".format(version_label, ctx.arch))
            else:
                layers.append(":rpmdb_nginx_{}_{}_hummingbird".format(version_label, ctx.arch))
        return layers

    return _layers
