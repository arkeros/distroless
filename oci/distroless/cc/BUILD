load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("//oci:vex.bzl", "vex_document")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES")
load(":config.bzl", "CC_ARCHITECTURES", "CC_DISTROS", "cc_layers")

package(default_visibility = ["//visibility:public"])

vex_document(
    name = "vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/cc/BUILD",
    statements = GLIBC_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

vex_document(
    name = "debug_vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/cc/BUILD#debug",
    statements = BUSYBOX_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

# Hummingbird: ADR 0007's whole point on cc is real-zero on glibc-2.42-13.hum1
# (CVE-2026-5435/5450/5928 backported by Hummingbird). Empty document is a
# placeholder so the matrix can pass `vex` per-distro uniformly — populate
# only if the scanner DB ever surfaces a CVE we need to suppress, which
# would defeat the ADR's thesis and warrant a louder response than VEX.
vex_document(
    name = "vex_hummingbird",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/cc/BUILD#hummingbird",
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

# Debian cc layer: libc6 + libssl3t64 + libstdc++6. Closure refs (not `:data`)
# preserve `PackageMetadataInfo` for SBOM — alias targets don't propagate
# `applicable_licenses` through the gather_metadata aspect.
[
    flatten(
        name = "cc_{}_debian_layer".format(arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@debian//libc6/{}".format(arch),
            "@debian//libssl3t64/{}".format(arch),
            "@debian//libstdc++6/{}".format(arch),
        ],
    )
    for arch in CC_ARCHITECTURES["debian"]
]

# Hummingbird cc layer: RHEL-shape mapping of the Debian set —
#   libc6        → glibc (+ glibc-common for locale data referenced by glibc
#                  scripts and tzdata's POSIX TZ lookups)
#   libstdc++6   → libstdc++ (+ libgcc; libstdc++.so.6 dynlinks libgcc_s.so.1)
#   libssl3t64   → openssl-libs (libssl.so.3 + libcrypto.so.3)
# glibc-2.42-13.hum1 is the version Red Hat's CVE API names as carrying
# backports for CVE-2026-5435/5450/5928 — the three glibc CVEs Debian sid
# still has on its wontfix list. This layer is the proof point for ADR 0007.
[
    flatten(
        name = "cc_amd64_hummingbird_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@hummingbird//glibc/x86_64",
            "@hummingbird//glibc-common/x86_64",
            "@hummingbird//libgcc/x86_64",
            "@hummingbird//libstdc++/x86_64",
            "@hummingbird//openssl-libs/x86_64",
            "@hummingbird//zlib-ng-compat/x86_64",
        ],
    ),
]

[
    flatten(
        name = "cc_arm64_hummingbird_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@hummingbird//glibc/aarch64",
            "@hummingbird//glibc-common/aarch64",
            "@hummingbird//libgcc/aarch64",
            "@hummingbird//libstdc++/aarch64",
            "@hummingbird//openssl-libs/aarch64",
            "@hummingbird//zlib-ng-compat/aarch64",
        ],
    ),
]

# Full-package rpmdb per arch. cc inherits static's rpmdb via `base`, but the
# rpmdb at /usr/lib/sysimage/rpm/rpmdb.sqlite is a single sqlite file —
# layering a partial rpmdb on top would clobber static's row set. So pass
# both targets: aspect walks each flatten and dedupes by (package, arch),
# producing a single rpmdb listing all 8 packages (static's 3 + cc's 5).
[
    rpmdb_merge(
        name = "rpmdb_cc_" + arch + "_hummingbird",
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            ":cc_{}_hummingbird_layer".format(arch),
        ],
    )
    for arch in CC_ARCHITECTURES["hummingbird"]
]

[
    distroless_matrix(
        name = "cc",
        architectures = CC_ARCHITECTURES[distro],
        base = "//oci/distroless/static",
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES if distro == "debian" else [],
        debug_vex = [":debug_vex"] if distro == "debian" else [],
        distro = distro,
        ignore_cves = _IGNORE_CVES[distro],
        layers = cc_layers,
        vex = _VEX[distro],
    )
    for distro in CC_DISTROS
]

container_structure_test(
    name = "cc_test",
    size = "small",
    configs = ["test.yaml"],
    driver = "tar",
    image = ":cc_nonroot_amd64_debian.tar",
)

container_structure_test(
    name = "cc_hummingbird_test",
    size = "small",
    configs = ["test_hummingbird.yaml"],
    driver = "tar",
    image = ":cc_nonroot_amd64_hummingbird.tar",
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
