load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document", "vex_statement")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS", "NCURSES_FIXED_VEX_STATEMENTS")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES", "DEBUG_MODE")
load(":config.bzl", "BASH_ARCHITECTURES", "BASH_DISTROS", "bash_layers")

# Hummingbird arch dirs map differently than senku's debian-style names:
# senku amd64 -> rpm x86_64, senku arm64 -> rpm aarch64. Same convention as
# busybox in static/BUILD.
_HUMMINGBIRD_ARCH_MAP = {
    "amd64": "x86_64",
    "arm64": "aarch64",
}

package(default_visibility = ["//visibility:public"])

# rust-coreutils 0.8.0-1 carries three unfixed Highs in Debian sid:
# mkfifo (CVE-2026-35341), mkdir TOCTOU (CVE-2026-35352), chroot
# (CVE-2026-35368). The rust-coreutils multicall binary contains all
# subcommands, but our images only expose the symlinks enumerated in
# //oci/distroless/common:BUILD's COREUTILS_COMMANDS. mkfifo and chroot
# were never in that set; mkdir was removed alongside this VEX. None
# of the three subcommands is resolvable by name on PATH — invoking
# them requires the non-standard `/bin/coreutils <name>` multicall
# form, which neither the entrypoint nor any layered tooling uses.
# Drops the moment Debian ships a fix (`_cve_test_stale_vex` will
# turn red and force deletion).
RUST_COREUTILS_VEX_STATEMENTS = [
    vex_statement(
        expires = "2026-08-11",
        impact_statement = "mkfifo is not in COREUTILS_COMMANDS — no /usr/bin/mkfifo symlink to the rust-coreutils multicall.",
        justification = "vulnerable_code_not_in_execute_path",
        products = ["pkg:oci/bash"],
        status = "not_affected",
        vulnerability = "CVE-2026-35341",
    ),
    vex_statement(
        expires = "2026-08-11",
        impact_statement = "mkdir was removed from COREUTILS_COMMANDS. Debug variants resolve mkdir to /busybox/mkdir (different implementation, not affected); non-debug variants don't expose mkdir on PATH.",
        justification = "vulnerable_code_not_in_execute_path",
        products = ["pkg:oci/bash"],
        status = "not_affected",
        vulnerability = "CVE-2026-35352",
    ),
    vex_statement(
        expires = "2026-08-11",
        impact_statement = "chroot is not in COREUTILS_COMMANDS — no /usr/bin/chroot symlink to the rust-coreutils multicall.",
        justification = "vulnerable_code_not_in_execute_path",
        products = ["pkg:oci/bash"],
        status = "not_affected",
        vulnerability = "CVE-2026-35368",
    ),
]

vex_document(
    name = "vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/bash/BUILD",
    statements = GLIBC_FIXED_VEX_STATEMENTS + NCURSES_FIXED_VEX_STATEMENTS + RUST_COREUTILS_VEX_STATEMENTS,
    timestamp = "2026-05-13T00:00:00Z",
)

vex_document(
    name = "debug_vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/bash/BUILD#debug",
    statements = BUSYBOX_FIXED_VEX_STATEMENTS + RUST_COREUTILS_VEX_STATEMENTS,
    timestamp = "2026-05-13T00:00:00Z",
)

# Hummingbird uses regular coreutils (no libsystemd0 split like Debian, so
# no rust-coreutils workaround needed) and ships /usr/bin/sh directly. No
# fixed-VEX statements yet — same posture as static/cc on hummingbird.
vex_document(
    name = "vex_hummingbird",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/bash/BUILD#hummingbird",
    statements = [],
    timestamp = "2026-05-15T00:00:00Z",
)

_IGNORE_CVES = {
    "debian": DEBIAN_WONTFIX_CVES,
    "hummingbird": [],
}

_VEX = {
    "debian": [":vex"],
    "hummingbird": [":vex_hummingbird"],
}

[
    distroless_matrix(
        name = "bash",
        architectures = BASH_ARCHITECTURES[distro],
        base = "//oci/distroless/cc",
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES if distro == "debian" else [],
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless bash image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/bash/BUILD",
        },
        debug_vex = [":debug_vex"] if distro == "debian" else [],
        distro = distro,
        entrypoint = ["/usr/bin/bash"],
        ignore_cves = _IGNORE_CVES[distro],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless bash image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/bash/BUILD",
        },
        layers = bash_layers,
        vex = _VEX[distro],
    )
    for distro in BASH_DISTROS
]

# Debian bash layer: rust-coreutils + symlinks (to avoid libsystemd0).
[
    flatten(
        name = arch + "_debian_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@debian//{}/{}".format("bash", arch),
            # for runfiles we need: env, sed, grep & awk
            # rust-coreutils provides /usr/bin/env without libsystemd0 dependency
            "@debian//{}/{}".format("rust-coreutils", arch),
            "//oci/distroless/common:coreutils_links",
            "@debian//{}/{}".format("sed", arch),
            "@debian//{}/{}".format("grep", arch),
            "@debian//{}/{}".format("mawk", arch),
            "//oci/distroless/common:awk",
        ],
    )
    for arch in BASH_ARCHITECTURES["debian"]
]

# Hummingbird bash layer: regular coreutils, no libsystemd0 split on RHEL
# so the rust-coreutils workaround isn't needed. /usr/bin/awk symlink is
# reused from common — Debian and Hummingbird both ship mawk at the same
# canonical path after rpm-extract's UsrMerge rewrite.
[
    flatten(
        name = arch + "_hummingbird_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@hummingbird//bash/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//coreutils/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//sed/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//grep/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//mawk/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            # Soname deps of the binaries above. All transitively in the
            # lockfile (pin walks rpm:requires) but flatten only ships
            # what's listed here, same posture as @debian's image composition:
            #   bash       -> libtinfo.so.6   (ncurses-libs)
            #   grep       -> libpcre2-8.so.0 (pcre2)
            #   sed        -> libselinux.so.1 (libselinux) -> libpcre2 + libsepol
            #              -> libattr.so.1   (libattr)
            #   coreutils  -> libselinux + libcap.so.2 (libcap) + libacl.so.1
            #                (libacl) -> libattr
            "@hummingbird//libacl/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//libattr/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//libcap/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//libselinux/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//libsepol/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//ncurses-libs/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "@hummingbird//pcre2/{}".format(_HUMMINGBIRD_ARCH_MAP[arch]),
            "//oci/distroless/common:awk",
        ],
    )
    for arch in BASH_ARCHITECTURES["hummingbird"]
]

# rpmdb tar per arch. Same pattern as cc/BUILD: aspect-walk static + cc +
# this layer to gather every package's header. Result overwrites the
# rpmdb inherited from cc with the bash superset.
[
    rpmdb_merge(
        name = "rpmdb_bash_" + arch + "_hummingbird",
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
            ":" + arch + "_hummingbird_layer",
        ],
    )
    for arch in BASH_ARCHITECTURES["hummingbird"]
]

[
    mirror_push(
        name = "bash" + mode,
        image = ":bash" + mode + "_nonroot_debian",
        repository = "bash",
        sbom = ":bash" + mode + "_nonroot_debian_sbom",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
    )
    for mode in DEBUG_MODE
]

container_structure_test(
    name = "bash_test",
    size = "small",
    configs = ["test.yaml"],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":bash_nonroot_amd64_debian.oci_layout",
    tags = ["requires-docker"],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
