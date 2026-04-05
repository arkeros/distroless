NGINX_VERSIONS = {
    "stable": "nginx_stable",
    "mainline": "nginx_mainline",
}

# version_label -> short_version tag
NGINX_TAGS = {
    "stable": "1.28",
    "mainline": "1.29",
}

NGINX_DISTROS = ["debian13"]

NGINX_ARCHITECTURES = {
    "debian13": ["amd64", "arm64"],
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
