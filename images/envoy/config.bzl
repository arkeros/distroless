"Configuration for envoy distroless images"

load("@envoy_tarballs//:versions.bzl", "VERSIONS")

ENVOY_DISTROS = ["hummingbird"]

ENVOY_ARCHITECTURES = {
    "hummingbird": ["amd64", "arm64"],
}

# ADR 0007 step 6 on a third runtime: envoy from envoyproxy's release
# assets on the cc-hummingbird base. The pins live in envoy.lock.json,
# which the Update Tarballs workflow moves to each line's newest release
# daily; adding or dropping a line is a hand edit of that file. Lines are
# listed oldest first.
#
# Every line upstream still patches is here. Envoy supports four at a time
# and cuts a security release across all of them at once, so a consumer
# holding back a line still gets the fix.
ENVOY_VERSIONS = {line["major"]: line["version"] for line in VERSIONS}

ENVOY_LINES = list(ENVOY_VERSIONS.keys())

# What `latest` answers to: the newest line listed, so a roll that appends
# the next one moves the tag with it. A bare `docker pull distroless.io/envoy`
# gets this build.
ENVOY_LATEST = ENVOY_LINES[-1]

# A Bazel repo name cannot carry a dot, so the lockfile's repos are keyed by
# the line with it stripped — `envoy_139_amd64` for 1.39. Target names keep
# the dot, as //images/python names `python3.11`.
def envoy_repo_line(line):
    return line.replace(".", "")

def envoy_layers(line):
    """Composition: static + (busybox if debug) + cc + envoy + one rpmdb.

    No base inheritance; everything is explicit. Branches on ctx.mode to
    add busybox + a busybox-aware rpmdb for the `_debug` variant. Envoy
    links glibc and nothing else — no libstdc++, and BoringSSL is static —
    so cc covers it with room to spare.
    """

    def _layers(ctx):
        layers = [
            "//images/static:static_{}_hummingbird_layer".format(ctx.arch),
        ]
        if ctx.mode == "_debug":
            layers.append("//images/static:busybox_{}_hummingbird_layer".format(ctx.arch))
        layers += [
            "//images/cc:cc_{}_hummingbird_layer".format(ctx.arch),
            "//images/envoy:envoy_{}_{}_hummingbird_layer".format(envoy_repo_line(line), ctx.arch),
        ]
        if ctx.mode == "_debug":
            layers.append("//images/envoy:rpmdb_envoy_debug_{}_{}_hummingbird".format(envoy_repo_line(line), ctx.arch))
        else:
            layers.append("//images/envoy:rpmdb_envoy_{}_{}_hummingbird".format(envoy_repo_line(line), ctx.arch))
        return layers

    return _layers
