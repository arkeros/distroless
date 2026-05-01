load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@rules_distroless//distroless:defs.bzl", "cacerts", "group", "home", "locale", "os_release", "passwd")
load("@tar.bzl", "tar")
load("//oci/distroless:platforms.bzl", "ALL_ARCHITECTURES", "ALL_DISTROS", "VERSIONS")
load(":variables.bzl", "COMPRESSION", "MTIME", "NOBODY", "NONROOT", "OS_RELEASE", "ROOT", "quote")

package(default_visibility = ["//visibility:public"])

COMMON_ARCHITECTURES = ALL_ARCHITECTURES

COMMON_DISTROS = ALL_DISTROS

tar(
    name = "rootfs",
    srcs = [],
    # args = [
    #     "--format",
    #     "gnutar",
    # ],
    compress = COMPRESSION,
    mtree = ["./ type=dir uid=0 gid=0 time=0.0"],
)

tar(
    name = "tmp",
    srcs = [],
    # # original tmp.tar was created on a gnutar, mimic that.
    # args = [
    #     "--format",
    #     "gnutar",
    # ],
    compress = COMPRESSION,
    mtree = ["./tmp gname=root uname=root time=1501783453.0 mode=1777 gid=0 uid=0 type=dir"],
)

# Create awk symlink for mawk
tar(
    name = "awk",
    mtree = [
        # Create awk symlink pointing to mawk for compatibility
        "./usr/bin/awk type=link link=/usr/bin/mawk",
    ],
)

# Create symlinks for rust-coreutils (avoids GNU coreutils which pulls in libsystemd0)
# rust-coreutils is a multicall binary at /bin/coreutils
COREUTILS_COMMANDS = [
    "env",
    "uname",
    "dirname",
    "basename",
    "cat",
    "cp",
    "ls",
    "mkdir",
    "mv",
    "rm",
    "rmdir",
    "pwd",
    "readlink",
    "realpath",
    "touch",
    "tr",
    "wc",
    "head",
    "tail",
    "sort",
    "uniq",
    "cut",
    "tee",
    "sleep",
    "date",
    "whoami",
    "id",
    "echo",
    "printf",
    "true",
    "false",
    "test",
    "[",
]

tar(
    name = "coreutils_links",
    mtree = [
        "./usr/bin/{} type=link link=/bin/coreutils".format(cmd)
        for cmd in COREUTILS_COMMANDS
    ],
)

[
    os_release(
        name = "os_release_%s" % dist,
        content = {
            key: quote(value.format(
                CODENAME = codename,
                VERSION = version,
            ))
            for (key, value) in OS_RELEASE.items()
        },
        time = MTIME,
    )
    for (dist, codename, version) in VERSIONS
]

[
    locale(
        name = "locale_%s_%s" % (distro, arch),
        charset = "C.utf8",
        package = "@{}//{}/{}:data".format(distro, "libc-bin", arch),
        time = MTIME,
    )
    for arch in COMMON_ARCHITECTURES
    for distro in COMMON_DISTROS
]

[
    cacerts(
        name = "cacerts_%s_%s" % (distro, arch),
        package = "@{}//{}/{}:data".format(distro, "ca-certificates", arch),
        time = MTIME,
    )
    for arch in COMMON_ARCHITECTURES
    for distro in COMMON_DISTROS
]

# create /etc/group with the root, tty, and staff groups
group(
    name = "group",
    entries = [
        {
            "name": "root",  # root_group
            "gid": ROOT,
            "password": "x",
        },
        {
            "name": "nobody",  # nobody_group
            "gid": NOBODY,
            "password": "x",
        },
        {
            "name": "tty",  # tty_group
            "gid": 5,
            "password": "x",
        },
        {
            "name": "staff",  # staff_group
            "gid": 50,
            "password": "x",
        },
        {
            "name": "nonroot",  # nonroot_group
            "gid": NONROOT,
            "password": "x",
        },
    ],
    time = MTIME,
)

passwd(
    name = "passwd",
    entries = [
        {
            "gecos": ["root"],
            "gid": ROOT,
            "shell": "/sbin/nologin",
            "home": "/root",
            "uid": ROOT,
            "password": "x",
            "username": "root",
        },
        {
            "gecos": ["nobody"],
            "gid": NOBODY,
            "home": "/nonexistent",
            "shell": "/sbin/nologin",
            "uid": NOBODY,
            "password": "x",
            "username": "nobody",
        },
        {
            "gecos": ["nonroot"],
            "gid": NONROOT,
            "home": "/home/nonroot",
            "shell": "/sbin/nologin",
            "uid": NONROOT,
            "password": "x",
            "username": "nonroot",
        },
    ],
)

home(
    name = "home",
    dirs = [
        {
            "home": "/root",
            "uid": ROOT,
            "gid": ROOT,
            "mode": 700,
        },
        {
            "home": "/home",
            "uid": ROOT,
            "gid": ROOT,
            "mode": 755,
        },
        {
            "home": "/home/nonroot",
            "uid": NONROOT,
            "gid": NONROOT,
            "mode": 700,
        },
    ],
)

bzl_library(
    name = "debian_fixed_vex",
    srcs = ["debian_fixed_vex.bzl"],
    deps = ["//oci:vex"],
)

bzl_library(
    name = "variables",
    srcs = ["variables.bzl"],
)
