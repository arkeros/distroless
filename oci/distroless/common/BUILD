load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@rules_distroless//distroless:defs.bzl", "cacerts", "group", "home", "locale", "os_release", "passwd")
load("@tar.bzl", "tar")
load("//oci:created_timestamp.bzl", "debian_created_timestamp", "hummingbird_created_timestamp")
load("//oci/distroless:platforms.bzl", "ALL_ARCHITECTURES", "ALL_DISTROS", "VERSIONS")
load(":variables.bzl", "COMPRESSION", "MTIME", "NOBODY", "NONROOT", "OS_RELEASE_BY_DISTRO", "ROOT", "quote")

package(default_visibility = ["//visibility:public"])

# Per-.deb BUILD template consumed by `apt.install(package_template = ...)`.
# Extends the upstream rules_distroless template with a `package_metadata`
# target (so SBOM generators see each .deb's purl) and a `dpkg_statusd`
# target (so syft/grype can catalog Debian packages on the runtime image
# filesystem, per https://github.com/GoogleContainerTools/distroless/blob/main/PACKAGE_METADATA.md).
exports_files(["package.BUILD.tmpl"])

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

# Companion to per-package `dpkg_statusd` outputs (declared by our
# //oci/distroless/common:package.BUILD.tmpl). Upstream rules_distroless's
# dpkg_statusd.sh emits only file entries (./var/lib/dpkg/status.d/<pkg> +
# .md5sums), no parent directory entries. Vanilla `tar -x` synthesizes
# parents implicitly, but syft's stereoscope (image layer indexer) treats
# orphan files as invisible — its dpkg cataloger then reports zero
# packages and `grype <image>` returns false negatives on every Debian CVE.
# This layer ships the parent dirs once per image so the per-package
# entries become discoverable.
tar(
    name = "dpkg_status_d_dirs",
    srcs = [],
    compress = COMPRESSION,
    mtree = [
        "./var type=dir uid=0 gid=0 mode=0755 time=" + MTIME,
        "./var/lib type=dir uid=0 gid=0 mode=0755 time=" + MTIME,
        "./var/lib/dpkg type=dir uid=0 gid=0 mode=0755 time=" + MTIME,
        "./var/lib/dpkg/status.d type=dir uid=0 gid=0 mode=0755 time=" + MTIME,
    ],
)

# Create awk symlink for mawk
tar(
    name = "awk",
    mtree = [
        # Create awk symlink pointing to mawk for compatibility
        "./usr/bin/awk type=link link=/usr/bin/mawk",
    ],
)

# /etc/os-release -> /usr/lib/os-release per os-release(5). Debian's
# base-files package ships this symlink for free; Hummingbird's package
# set doesn't include a `setup` / `filesystem` rpm (deliberate, see ADR
# 0007), so we synthesize it senku-side.
tar(
    name = "etc_os_release_symlink",
    mtree = [
        "./etc/os-release type=link link=/usr/lib/os-release",
    ],
)

# UsrMerge root symlinks for Hummingbird-derived images. RHEL's `filesystem`
# rpm ships /lib, /lib64, /bin, /sbin as symlinks into /usr — these are
# *load-bearing*: every RHEL ELF binary is compiled with interpreter
# `/lib64/ld-linux-x86-64.so.2` (or aarch64 equivalent), and libgcc places
# libgcc_s.so.1 under /lib64. Without these symlinks, no upstream-built
# binary can find its dynamic linker on a Hummingbird-derived distroless.
#
# Debian images don't need this — their packages are pre-merged via
# `mergedusr = True` in apt.install (mergedusr.dh symlinks everything into
# /usr at .deb-install time). For RHEL-shape rpms the merge has to happen
# at image-composition time instead.
tar(
    name = "usrmerge_symlinks_hummingbird",
    mtree = [
        "./bin type=link link=usr/bin",
        "./sbin type=link link=usr/bin",
        "./lib type=link link=usr/lib",
        "./lib64 type=link link=usr/lib64",
    ],
)

# Create symlinks for rust-coreutils (avoids GNU coreutils which pulls in libsystemd0)
# rust-coreutils is a multicall binary at /bin/coreutils.
#
# `mkdir` is intentionally omitted: rust-coreutils 0.8.0-1 carries an
# unfixed TOCTOU (CVE-2026-35352). Debug variants fall back to
# /busybox/mkdir (different implementation, not affected); non-debug
# images simply don't expose mkdir on PATH. Companion VEX in
# //oci/distroless/bash:vex documents the non-reachability.
COREUTILS_COMMANDS = [
    "env",
    "uname",
    "dirname",
    "basename",
    "cat",
    "cp",
    "ls",
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
            for (key, value) in OS_RELEASE_BY_DISTRO[dist].items()
        },
        time = MTIME,
    )
    for (dist, codename, version) in VERSIONS
]

# locale + cacerts here are Debian-shaped: locale extracts charmaps from
# `libc-bin`, cacerts from Debian's `ca-certificates` package. Hummingbird's
# equivalents live at different paths inside differently-named rpms
# (glibc-common / ca-certificates with the RHEL ca-bundle layout) and need
# their own extractor. Restrict this iteration to Debian until those are wired.
_LOCALE_CACERTS_DISTROS = ["debian"]

[
    locale(
        name = "locale_%s_%s" % (distro, arch),
        charset = "C.utf8",
        package = "@{}//{}/{}:data".format(distro, "libc-bin", arch),
        time = MTIME,
    )
    for arch in COMMON_ARCHITECTURES
    for distro in _LOCALE_CACERTS_DISTROS
]

[
    cacerts(
        name = "cacerts_%s_%s" % (distro, arch),
        package = "@{}//{}/{}:data".format(distro, "ca-certificates", arch),
        time = MTIME,
    )
    for arch in COMMON_ARCHITECTURES
    for distro in _LOCALE_CACERTS_DISTROS
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

# RFC 3339 `created` timestamps shared across image families that consume
# the same upstream-snapshot anchor. Each evaluates to a one-file label
# consumable by `oci_image(created = ...)`. See //oci:created_timestamp.bzl
# and ADR 0007 §"Build horizon" amendment for rationale.
hummingbird_created_timestamp(
    name = "created_hummingbird",
    lock = "//:hummingbird.lock.json",
)

debian_created_timestamp(
    name = "created_debian",
    lock = "//oci/distroless:debian.lock.json",
)

bzl_library(
    name = "debian_fixed_vex",
    srcs = ["debian_fixed_vex.bzl"],
)

bzl_library(
    name = "variables",
    srcs = ["variables.bzl"],
)
