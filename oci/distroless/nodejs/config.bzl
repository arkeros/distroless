"Configuration for nodejs distroless images"

NODEJS_DISTROS = ["hummingbird"]

NODEJS_ARCHITECTURES = {
    "hummingbird": ["amd64", "arm64"],
}

# ADR 0007 step 6 ships 24/26 from nodejs.org tarballs on the cc-hummingbird
# base. Refresh by bumping these majors plus the matching http_archive
# entries in //bazel/include/oci.MODULE.bazel (versions + SHASUMS256 from
# https://nodejs.org/dist/v<version>/SHASUMS256.txt).
#
# Only lines still receiving upstream security releases belong here. The 20
# line was dropped when it went EOL: its last release (20.20.2, 2026-03-24)
# carries CVE-2026-58043 with no fix coming, and `_cve_test` has no honest
# way to pass on a runtime nobody patches any more.
NODEJS_VERSIONS = {
    "24": "24.19.0",
    "26": "26.6.0",
}

NODEJS_MAJOR_VERSIONS = list(NODEJS_VERSIONS.keys())

def nodejs_layers(major_version):
    """Composition: static + (busybox if debug) + cc + nodejs + one rpmdb.

    No base inheritance; everything is explicit. Branches on ctx.mode to
    add busybox + a busybox-aware rpmdb for the `_debug` variant.
    """

    def _layers(ctx):
        layers = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(ctx.arch),
        ]
        if ctx.mode == "_debug":
            layers.append("//oci/distroless/static:busybox_{}_hummingbird_layer".format(ctx.arch))
        layers += [
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(ctx.arch),
            "//oci/distroless/nodejs:nodejs_{}_{}_hummingbird_layer".format(major_version, ctx.arch),
        ]
        if ctx.mode == "_debug":
            layers.append("//oci/distroless/nodejs:rpmdb_nodejs_debug_{}_{}_hummingbird".format(major_version, ctx.arch))
        else:
            layers.append("//oci/distroless/nodejs:rpmdb_nodejs_{}_{}_hummingbird".format(major_version, ctx.arch))
        return layers

    return _layers
