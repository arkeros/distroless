"Configuration for python distroless images"

PYTHON_DISTROS = ["hummingbird"]

PYTHON_ARCHITECTURES = {
    "hummingbird": ["amd64", "arm64"],
}

# minor -> (interpreter rpm, upstream version). Hummingbird packages its
# default line as Fedora's main `python3` and every other supported line as
# a flat `python3.NN`; each has a `-libs` companion with libpython and the
# stdlib. Every line here is one upstream still ships security releases for
# and Hummingbird still rebuilds; 3.10 is in neither distro we compose from,
# and upstream drops it on 2026-10-31 anyway.
#
# The version is what the tags say, so `python_<minor>_lock_version_test`
# holds it equal to the lockfile: a re-pin that moves a line turns that test
# red rather than publishing a tag that lies. Refresh by bumping the version
# here after `bazel run @hummingbird//:pin`.
PYTHON_VERSIONS = {
    "3.11": ("python3.11", "3.11.16"),
    "3.12": ("python3.12", "3.12.14"),
    "3.13": ("python3.13", "3.13.15"),
    "3.14": ("python3", "3.14.7"),
}

PYTHON_MINOR_VERSIONS = list(PYTHON_VERSIONS.keys())

# What `latest` answers to: the newest line listed, so a roll that appends
# the next one moves the tag with it.
PYTHON_LATEST = PYTHON_MINOR_VERSIONS[-1]

def python_interpreter_rpm(minor):
    return PYTHON_VERSIONS[minor][0]

def python_version(minor):
    return PYTHON_VERSIONS[minor][1]

def python_layers(minor):
    """Layer composition for one python line.

    static + (busybox if debug) + cc + shared python libs + this line's
    interpreter (+ its python3 symlink) + one rpmdb. No base inheritance; everything is explicit. The shared-libs layer is
    the same blob under every line of an architecture. Branches on
    ctx.mode to add busybox + a busybox-aware rpmdb for the `_debug`
    variant.
    """

    def _layers(ctx):
        layers = [
            "//images/static:static_{}_hummingbird_layer".format(ctx.arch),
        ]
        if ctx.mode == "_debug":
            layers.append("//images/static:busybox_{}_hummingbird_layer".format(ctx.arch))
        layers += [
            "//images/cc:cc_{}_hummingbird_layer".format(ctx.arch),
            "//images/python:python_libs_{}_hummingbird_layer".format(ctx.arch),
            "//images/python:python_{}_{}_hummingbird_layer".format(minor, ctx.arch),
        ]
        if python_interpreter_rpm(minor) != "python3":
            layers.append("//images/python:python3_symlink_{}".format(minor))
        if ctx.mode == "_debug":
            layers.append("//images/python:rpmdb_python_debug_{}_{}_hummingbird".format(minor, ctx.arch))
        else:
            layers.append("//images/python:rpmdb_python_{}_{}_hummingbird".format(minor, ctx.arch))
        return layers

    return _layers
