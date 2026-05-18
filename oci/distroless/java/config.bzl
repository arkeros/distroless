"Configuration for java distroless images"

JAVA_DISTROS = ["hummingbird"]

JAVA_ARCHITECTURES = {
    "hummingbird": ["amd64", "arm64"],
}

# LTS-only policy. Auto-roll: latest 3 LTS, no non-LTS, no EOL-trajectory.
# Adoptium's LTS list is [8, 11, 17, 21, 25] (per `/v3/info/available_releases`).
# 8 + 11 are deliberately excluded — same posture as Google Distroless's
# current java/BUILD ([17, 21, 25]) — too far down the EOL slope to absorb
# the per-quarter CVE-rebuild promise on a one-maintainer repo. When 29
# ships (~2027), 17 rolls off and 29 takes its place here.
#
# Refresh by bumping (version, build) plus the matching http_archive entries
# in //bazel/include/oci.MODULE.bazel.
#
# `version` is the Adoptium version triplet that appears in CPE strings;
# `build` is Adoptium's build counter appended after the `+` in the release
# name (e.g. `jdk-21.0.11+10`).
JAVA_VERSIONS = {
    "17": ("17.0.19", "10"),
    "21": ("21.0.11", "10"),
    "25": ("25.0.3", "9"),
}

JAVA_MAJOR_VERSIONS = list(JAVA_VERSIONS.keys())

# Map senku arch (amd64/arm64) -> rpm arch (x86_64/aarch64). Same shape as
# the nodejs/BUILD. Used only for rpmdb_merge composition; Temurin tarball
# naming (linux x64 vs linux aarch64) is handled inside the http_archive
# URL template in oci.MODULE.bazel.
HUMMINGBIRD_ARCH_MAP = {
    "amd64": "x86_64",
    "arm64": "aarch64",
}

def java_layers(major_version):
    """Composition: static + (busybox if debug) + cc + temurin + one rpmdb.

    Release ships the JRE layer at /jre/; debug substitutes the JDK
    layer at the same /jre/ path so operators get jstack/jcmd/jmap/javac
    via `kubectl exec` without having to special-case the variant in
    their command/args overrides. The JDK is a strict superset of the
    JRE (same `bin/java`, same `lib/server/libjvm.so`) so the entrypoint
    and SBOM-level CPE component are unchanged across modes.

    No base inheritance; everything is explicit. Branches on ctx.mode to
    add busybox + the busybox-aware rpmdb for the `_debug` variant.
    """

    def _layers(ctx):
        layers = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(ctx.arch),
        ]
        if ctx.mode == "_debug":
            layers.append("//oci/distroless/static:busybox_{}_hummingbird_layer".format(ctx.arch))
        layers.append("//oci/distroless/cc:cc_{}_hummingbird_layer".format(ctx.arch))
        if ctx.mode == "_debug":
            layers += [
                "//oci/distroless/java:java_jdk_{}_{}_hummingbird_layer".format(major_version, ctx.arch),
                "//oci/distroless/java:rpmdb_java_debug_{}_{}_hummingbird".format(major_version, ctx.arch),
            ]
        else:
            layers += [
                "//oci/distroless/java:java_jre_{}_{}_hummingbird_layer".format(major_version, ctx.arch),
                "//oci/distroless/java:rpmdb_java_{}_{}_hummingbird".format(major_version, ctx.arch),
            ]
        return layers

    return _layers
