NGINX_VERSIONS = {
    "stable": "nginx_stable",
    "mainline": "nginx_mainline",
}

# version_label -> short_version tag
NGINX_TAGS = {
    "stable": "1.30",
    "mainline": "1.29",
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

NGINX_ARCHITECTURES = {
    "debian": ["amd64", "arm64"],
}

def nginx_layers(version_label):
    """Returns a layers callback for the given nginx version.

    Args:
        version_label: "stable" or "mainline"
    """

    def _layers(ctx):
        return [
            ":{}_{}_{}_layer".format(version_label, ctx.arch, ctx.distro),
            ":nginx_conf_layer",
        ]

    return _layers
