"Configuration for python distroless images"

PYTHON_DISTROS = ["hummingbird", "debian"]

PYTHON_ARCHITECTURES = {
    "hummingbird": ["amd64", "arm64"],
    "debian": ["amd64", "arm64"],
}

# distro -> minor -> (interpreter package, upstream version).
#
# Hummingbird packages its default line as Fedora's main `python3` and every
# other supported line as a flat `python3.NN`; each has a `-libs` companion
# with libpython and the stdlib. Every line there is one upstream still ships
# security releases for and Hummingbird still rebuilds; 3.10 is in neither
# distro we compose from, and upstream drops it on 2026-10-31 anyway.
#
# Debian sid carries only the lines it is currently building — 3.13 and 3.14
# at the pinned snapshot. 3.15 is there too but as `3.15.0~rc2-1`, and a tag
# that names a release candidate would have to move again at final release.
# The interpreter package is the stem the four Debian binary packages derive
# from; it is never plain `python3`, so every Debian line gets the
# `/usr/bin/python3` symlink layer, the same way Hummingbird's flat lines do.
#
# The version is what the tags say, so `python_<minor>_<distro>_lock_version_test`
# holds it equal to the lockfile: a re-pin that moves a line turns that test
# red rather than publishing a tag that lies. Refresh by bumping the version
# here after `bazel run @hummingbird//:pin` or `bazel run @debian//:lock`.
PYTHON_VERSIONS = {
    "hummingbird": {
        "3.11": ("python3.11", "3.11.16"),
        "3.12": ("python3.12", "3.12.14"),
        "3.13": ("python3.13", "3.13.15"),
        "3.14": ("python3", "3.14.7"),
    },
    "debian": {
        "3.13": ("python3.13", "3.13.15"),
        "3.14": ("python3.14", "3.14.7"),
    },
}

def python_minor_versions(distro):
    """The lines a distro publishes, oldest first."""
    return list(PYTHON_VERSIONS[distro].keys())

def python_latest(distro):
    """What `latest` answers to on a distro: the newest line listed, so a
    roll that appends the next one moves the tag with it."""
    return python_minor_versions(distro)[-1]

def python_interpreter_package(distro, minor):
    return PYTHON_VERSIONS[distro][minor][0]

def python_version(distro, minor):
    return PYTHON_VERSIONS[distro][minor][1]

def python_needs_symlink(distro, minor):
    """Whether the line has to be given `/usr/bin/python3` itself.

    Only Hummingbird's default line ships the symlink in its own package;
    every flat `python3.NN` rpm and every Debian `python3.NN-minimal` deb
    ships the versioned binary alone.
    """
    return python_interpreter_package(distro, minor) != "python3"

# Every line that needs a `/usr/bin/python3` symlink layer, across distros.
# One tar per minor, shared by whichever distros ask for it: the mtree is
# the same string either way, so 3.13 wants one entry and not two. A dict
# keyed by minor is how Starlark spells "unique" without a set type.
PYTHON_SYMLINK_MINOR_VERSIONS = sorted({
    minor: None
    for distro in PYTHON_DISTROS
    for minor in python_minor_versions(distro)
    if python_needs_symlink(distro, minor)
}.keys())

def python_layers(minor):
    """Layer composition for one python line.

    static + (busybox if debug) + cc + shared python libs + this line's
    interpreter (+ its python3 symlink), and on Hummingbird one merged
    rpmdb. No base inheritance; everything is explicit. The shared-libs
    layer is the same blob under every line of an architecture. Branches on
    ctx.mode to add busybox + a busybox-aware rpmdb for the `_debug`
    variant, and on ctx.distro for the package source. Debian needs no
    rpmdb layer: each .deb carries its own dpkg status.d entry.
    """

    def _layers(ctx):
        layers = [
            "//images/static:static_{}_{}_layer".format(ctx.arch, ctx.distro),
        ]
        if ctx.mode == "_debug":
            layers.append("//images/static:busybox_{}_{}_layer".format(ctx.arch, ctx.distro))
        layers += [
            "//images/cc:cc_{}_{}_layer".format(ctx.arch, ctx.distro),
            "//images/python:python_libs_{}_{}_layer".format(ctx.arch, ctx.distro),
            "//images/python:python_{}_{}_{}_layer".format(minor, ctx.arch, ctx.distro),
        ]
        if python_needs_symlink(ctx.distro, minor):
            layers.append("//images/python:python3_symlink_{}".format(minor))
        if ctx.distro == "hummingbird":
            debug = "_debug" if ctx.mode == "_debug" else ""
            layers.append("//images/python:rpmdb_python{}_{}_{}_hummingbird".format(debug, minor, ctx.arch))
        return layers

    return _layers
