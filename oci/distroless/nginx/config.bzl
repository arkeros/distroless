NGINX_VERSIONS = {
    "stable": "nginx_stable",
    "mainline": "nginx_mainline",
}

# version_label -> short_version tag
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

# Hummingbird ships a single nginx version per snapshot (1.30.x in the
# current pin). pin picks the highest, which corresponds to upstream
# nginx's mainline channel — so the hummingbird-derived image only
# composes the mainline variant. Adding a hummingbird-stable image
# would require version-constrained pinning (`nginx<1.29`), out of
# scope for this slice. Stable stays debian-only until then.
NGINX_CHANNEL_DISTROS = {
    "stable": ["debian"],
    "mainline": ["debian", "hummingbird"],
}

NGINX_ARCHITECTURES = {
    "debian": ["amd64", "arm64"],
    "hummingbird": ["amd64", "arm64"],
}

def nginx_layers(version_label):
    """Returns a layers callback for the given nginx version.

    Args:
        version_label: "stable" or "mainline"
    """

    def _layers(ctx):
        layers = [
            ":{}_{}_{}_layer".format(version_label, ctx.arch, ctx.distro),
            ":nginx_conf_layer",
        ]
        if ctx.distro == "hummingbird":
            # nginx adds nginx-core + nginx-filesystem on top of cc. The
            # rpmdb merges everything from static + cc + this layer so
            # syft sees the full package set.
            layers.append(":rpmdb_nginx_{}_{}_hummingbird".format(version_label, ctx.arch))
        return layers

    return _layers
