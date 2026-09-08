load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("@tar.bzl", "tar")
load("//images:matrix.bzl", "distroless_matrix")
load("//images:platforms.bzl", "HUMMINGBIRD_ARCH_MAP")
load("//images/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEFAULT_DEBUG_ENV", "DEFAULT_ENV", "NONROOT")
load("//oci:vex.bzl", "vex_document")
load(":config.bzl", "PGDATA", "POSTGRES_ARCHITECTURES", "POSTGRES_DISTROS", "POSTGRES_VERSIONS", "postgres_layers", "postgres_lines", "postgres_package")

package(default_visibility = ["//visibility:public"])

_HUMMINGBIRD_LINES = postgres_lines("hummingbird")

# The libraries postgres links that cc does not already carry. Read off
# `rpm -qR` of the three postgresql rpms (rules_rpm's pin walks the same
# requires into the lockfile), and held honest by the elf_needs test that
# every image here generates:
#
#   postgres  -> libicuuc, libicui18n (ICU collations, not optional: the
#                server links them whatever locale provider a cluster uses)
#             -> libgssapi_krb5 (krb5-libs), libldap (openldap, which links
#                libsasl2 -> cyrus-sasl-lib and libevent)
#             -> libpam.so.0 (pam-libs — the soname package, not the full
#                `pam`, which is PAM's modules and configuration)
#             -> libsystemd (sd_notify readiness), libxml2.so.16, liblz4,
#                libzstd, libpq.so.private18-5 (postgresql-private-libs)
#   psql      -> libreadline -> libtinfo (ncurses-libs)
#   contrib   -> libxslt (xml2), libuuid (uuid-ossp)
#
# glibc-langpack-en is not linked by anything; it is the compiled
# en_US.utf8 without which `initdb --locale=en_US.utf8` fails. See
# plans/0002 for why Debian cannot have an equivalent.
#
# The second group is what those in turn load, every one of them named by
# the elf_needs test on the first attempt: krb5 wants libcom_err,
# libkeyutils, libselinux and libverto; PAM wants libaudit and libeconf;
# Cyrus SASL wants libcrypt and, for its own local password database,
# libgdbm; openldap's libslapi wants libltdl; and contrib's uuid-ossp wants
# libossp-uuid, which is the `uuid` rpm rather than `libuuid`. One more round
# of the same test added libcap-ng (libaudit) and pcre2 (libselinux). They
# total about 1.5 MB, so they are shipped rather than allow-listed — every
# ELF in this image loads, and the test has no exceptions to get stale.
_POSTGRES_LIBS = [
    "audit-libs",
    "cyrus-sasl-lib",
    "gdbm-libs",
    "glibc-langpack-en",
    "keyutils-libs",
    "krb5-libs",
    "libcap-ng",
    "libcom_err",
    "libeconf",
    "libevent",
    "libicu",
    "libselinux",
    "libtool-ltdl",
    "libuuid",
    "libverto",
    "libxcrypt",
    "libxml2-16",
    "libxslt",
    "libzstd",
    "lz4-libs",
    "ncurses-libs",
    "openldap",
    "pam-libs",
    "pcre2",
    "readline",
    "systemd-libs",
    "uuid",
]

# The server, the client tools, the contrib extensions and the private libpq
# they share, plus the libraries above. One layer per (line, arch): the two
# lines share no bytes — each ships its own /usr/bin/postgres — so there is
# nothing to lift into a shared layer the way python's stdlib libraries are.
[
    flatten(
        name = "postgres_{}_{}_hummingbird_layer".format(line, arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@hummingbird//{}{}/{}".format(
                postgres_package("hummingbird", line),
                suffix,
                HUMMINGBIRD_ARCH_MAP[arch],
            )
            for suffix in [
                "",
                "-server",
                "-contrib",
                "-private-libs",
            ]
        ] + [
            "@hummingbird//{}/{}".format(
                lib,
                HUMMINGBIRD_ARCH_MAP[arch],
            )
            for lib in _POSTGRES_LIBS
        ],
    )
    for line in _HUMMINGBIRD_LINES
    for arch in POSTGRES_ARCHITECTURES["hummingbird"]
]

# /bin/sh, for the debug variant only.
#
# Every other family's debug image is content with busybox under /busybox
# and that directory on PATH, because a human types the command. Postgres
# does not: `initdb` and `pg_ctl` locate their helpers through popen(), the
# server runs `archive_command`, `restore_command` and `COPY ... PROGRAM`
# through system(), and both spell the shell `/bin/sh` absolutely — PATH is
# never consulted. Without this symlink the debug image has a shell that
# postgres itself cannot reach, and `initdb` fails with the memorable
# `program "postgres" is needed by initdb but was not found in the same
# directory`, which is popen() failing rather than anything being missing.
#
# `./usr/bin/sh`, not `./bin/sh`: these images are usrmerged (`./bin ->
# usr/bin` in the static layer), so a `./bin/sh` entry would write through
# a symlink.
tar(
    name = "sh_symlink_layer",
    mtree = [
        "./usr/bin/sh uid=0 gid=0 mode=0755 time=0.0 type=link link=/usr/bin/busybox",
    ],
)

# PGDATA, owned by the nonroot uid and 0700 — postgres refuses to start on a
# data directory with wider permissions or a different owner, and there is no
# entrypoint script here to chown one at boot. Shared by every variant: the
# root images get the same directory, which is harmless since uid 0 can write
# it anyway, and keeps this one blob rather than one per user variant.
#
# `time=0.0` on the directory entries, for the reason nginx_conf_layer
# records: without it bsdtar stamps the build's own clock and the layer, the
# image config and the index digest differ per machine.
tar(
    name = "pgdata_layer",
    mtree = [
        "./var/lib/postgresql uid={uid} gid={uid} mode=0700 time=0.0 type=dir".format(uid = NONROOT),
        ".{path} uid={uid} gid={uid} mode=0700 time=0.0 type=dir".format(
            path = PGDATA,
            uid = NONROOT,
        ),
    ],
)

# rpmdb_merge — one sqlite covering static + cc + postgres. With no base
# inheritance this is the only rpmdb in the release image.
[
    rpmdb_merge(
        name = "rpmdb_postgres_{}_{}_hummingbird".format(line, arch),
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
            ":postgres_{}_{}_hummingbird_layer".format(line, arch),
        ],
    )
    for line in _HUMMINGBIRD_LINES
    for arch in POSTGRES_ARCHITECTURES["hummingbird"]
]

# Debug rpmdb adds busybox rows for the _debug variant.
[
    rpmdb_merge(
        name = "rpmdb_postgres_debug_{}_{}_hummingbird".format(line, arch),
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/static:busybox_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
            ":postgres_{}_{}_hummingbird_layer".format(line, arch),
        ],
    )
    for line in _HUMMINGBIRD_LINES
    for arch in POSTGRES_ARCHITECTURES["hummingbird"]
]

# Hummingbird's suppression list is empty by design — that a glibc-bearing
# image needs none is ADR 0007's thesis. The document stays wired so the
# first statement is one line.
[
    vex_document(
        name = "vex_hummingbird_" + line,
        author = "rafael@arquero.cat",
        id = "https://github.com/arkeros/distroless/blob/main/images/postgres/BUILD#hummingbird-" + line,
        statements = [],
        timestamp = "2026-09-08T00:00:00Z",
    )
    for line in _HUMMINGBIRD_LINES
]

# `postgres` refuses to run as uid 0, so the root index of this family is a
# matrix artefact rather than something to pull; the nonroot tags are the
# tags, and they are what mirror_push publishes. LANG is en_US.utf8 because
# that is the locale glibc-langpack-en supplies and the one a cluster
# initdb'd elsewhere is most likely to be in; a cluster built with
# `--locale-provider=builtin` ignores it.
[
    distroless_matrix(
        name = "postgres" + line,
        architectures = POSTGRES_ARCHITECTURES[distro],
        created = "//images/common:created_" + distro,
        debug_entrypoint = ["/busybox/sh"],
        debug_env = DEFAULT_DEBUG_ENV,
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro],
        debug_layers = postgres_layers(line),
        distro = distro,
        entrypoint = ["/usr/bin/postgres"],
        env = dict(
            DEFAULT_ENV,
            LANG = "en_US.utf8",
            PGDATA = PGDATA,
        ),
        index_annotations = {
            "org.opencontainers.image.description": "Distroless PostgreSQL {} image".format(line),
            "org.opencontainers.image.source": "https://github.com/arkeros/distroless/blob/main/images/postgres/BUILD",
        },
        layers = postgres_layers(line),
        vex = [":vex_hummingbird_" + line],
    )
    for distro in POSTGRES_DISTROS
    for line in postgres_lines(distro)
]

# Behavioural, not a file census: the release image proves it can build a
# cluster at the locale it ships, and the debug image proves a cluster runs
# and round-trips a row. One line only for now; the loop is over lines so
# the next one is free.
[
    container_structure_test(
        name = "postgres{}_test".format(line),
        size = "large",
        configs = ["test.yaml"],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":postgres{}_nonroot_amd64_hummingbird.oci_layout".format(line),
        tags = ["requires-docker"],
    )
    for line in _HUMMINGBIRD_LINES
]

[
    container_structure_test(
        name = "postgres{}_debug_test".format(line),
        size = "large",
        configs = ["test_debug.yaml"],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":postgres{}_debug_nonroot_amd64_hummingbird.oci_layout".format(line),
        tags = ["requires-docker"],
    )
    for line in _HUMMINGBIRD_LINES
]

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
