"""The postgres images: their lines, distros, architectures and layers."""

POSTGRES_DISTROS = ["hummingbird"]

POSTGRES_ARCHITECTURES = {
    "hummingbird": ["amd64", "arm64"],
}

# distro -> line -> (package stem, upstream version).
#
# Hummingbird packages its default line as the unsuffixed `postgresql*` and
# every other supported line as a flat `postgresql<major>*` — the same shape
# as its python lines, and the reason this is keyed by stem rather than
# composed from the line number. Both lines put their binaries in /usr/bin
# (`postgresql17` ships a real /usr/bin/psql, not an alternatives symlink),
# so PATH does not vary by line.
#
# The version is what the tags say, and `postgres_<line>_<distro>_lock_version_test`
# holds it equal to the lockfile: a repin that moves a line turns that test
# red rather than publishing a tag that lies. Refresh by bumping the version
# here after `bazel run @hummingbird//:pin`.
POSTGRES_VERSIONS = {
    "hummingbird": {
        "18": ("postgresql", "18.6"),
    },
}

# Where PGDATA lives, and the directory the image ships for it. Postgres
# refuses to start on a data directory it does not own and that is not 0700,
# so this is created here rather than left to the consumer's volume mount.
PGDATA = "/var/lib/postgresql/data"

def postgres_lines(distro):
    """The lines a distro publishes, oldest first."""
    return list(POSTGRES_VERSIONS[distro].keys())

def postgres_latest(distro):
    """What `latest` answers to on a distro.

    The newest line listed, so a roll that appends the next one moves the
    tag with it.
    """
    return postgres_lines(distro)[-1]

def postgres_package(distro, line):
    return POSTGRES_VERSIONS[distro][line][0]

def postgres_version(distro, line):
    return POSTGRES_VERSIONS[distro][line][1]

def postgres_layers(line):
    """Layer composition for one postgres line.

    static + (busybox if debug) + cc + this line's postgres + the PGDATA
    directory, and on Hummingbird one merged rpmdb. No base inheritance;
    everything is explicit. Branches on ctx.mode to add busybox + a
    busybox-aware rpmdb for the `_debug` variant.

    Args:
        line: the major line, e.g. "18".
    """

    def _layers(ctx):
        layers = [
            "//images/static:static_{}_{}_layer".format(ctx.arch, ctx.distro),
        ]
        if ctx.mode == "_debug":
            layers += [
                "//images/static:busybox_{}_{}_layer".format(ctx.arch, ctx.distro),
                # /bin/sh, which postgres's own tooling calls absolutely.
                # See the layer's comment in //images/postgres:BUILD.
                "//images/postgres:sh_symlink_layer",
            ]
        layers += [
            "//images/cc:cc_{}_{}_layer".format(ctx.arch, ctx.distro),
            "//images/postgres:postgres_{}_{}_{}_layer".format(line, ctx.arch, ctx.distro),
            "//images/postgres:pgdata_layer",
        ]
        if ctx.distro == "hummingbird":
            debug = "_debug" if ctx.mode == "_debug" else ""
            layers.append("//images/postgres:rpmdb_postgres{}_{}_{}_hummingbird".format(debug, line, ctx.arch))
        return layers

    return _layers
