load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("//oci:vex.bzl", "vex_document")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless:platforms.bzl", BUSYBOX_ARCHITECTURES = "ALL_ARCHITECTURES")
load("//oci/distroless/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES")
load(":busybox.bzl", "busybox_layer")
load(":config.bzl", "STATIC_ARCHITECTURES", "STATIC_DISTROS", "static_debug_layers", "static_layers")

package(default_visibility = ["//visibility:public"])

vex_document(
    name = "vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/static/BUILD",
    statements = GLIBC_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

vex_document(
    name = "debug_vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/static/BUILD#debug",
    statements = BUSYBOX_FIXED_VEX_STATEMENTS,
    timestamp = "2026-05-01T00:00:00Z",
)

# Hummingbird: no fixed-VEX statements yet (ADR 0007's competitive thesis is
# real-zero on these packages, not VEX-suppressed). Empty document is a
# placeholder so the matrix can pass `vex` per-distro uniformly.
vex_document(
    name = "vex_hummingbird",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/static/BUILD#hummingbird",
    statements = [],
    timestamp = "2026-05-15T00:00:00Z",
)

# Per-distro CVE allow-list and VEX. Each invocation of distroless_matrix
# below selects the right pair via _IGNORE_CVES[distro] / _VEX[distro].
# Hummingbird's `tzdata`/`ca-certificates`/`mailcap` carry no published CVEs,
# so both lists are empty; revisit once glibc-bearing images migrate.
_IGNORE_CVES = {
    "debian": DEBIAN_WONTFIX_CVES,
    "hummingbird": [],
}

_VEX = {
    "debian": [":vex"],
    "hummingbird": [":vex_hummingbird"],
}

_DEFAULT_PATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

# Debian flatten: per-arch label shape (`@debian//tzdata/amd64`). Uses the
# senku `cacerts_<distro>_<arch>` extraction since Debian's ca-certificates
# package needs path normalisation.
[
    flatten(
        name = "static_" + arch + "_debian_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@debian//{}/{}".format("base-files", arch),
            "@debian//{}/{}".format("netbase", arch),
            "@debian//{}/{}".format("tzdata", arch),
            "@debian//{}/{}".format("media-types", arch),
            "//oci/distroless/common:rootfs",
            "//oci/distroless/common:passwd",
            "//oci/distroless/common:home",
            "//oci/distroless/common:group",
            "//oci/distroless/common:tmp",
            "//oci/distroless/common:os_release_debian",
            "//oci/distroless/common:cacerts_debian_" + arch,
        ],
    )
    for arch in STATIC_ARCHITECTURES["debian"]
]

# Hummingbird flatten: ADR 0007's static composition. Three Hummingbird
# packages (tzdata, ca-certificates, mailcap — all noarch) + the senku-
# synthesized rootfs/passwd/home/group/tmp + ID=hummingbird os-release.
# No senku-side `cacerts` extractor needed: Hummingbird's ca-certificates
# rpm already ships /etc/ssl/certs/ca-certificates.crt as a Debian-compat
# symlink alongside its native /etc/pki/ca-trust layout, so the existing
# SSL_CERT_FILE env in the matrix call resolves correctly. rpmdb_merge is
# intentionally omitted while the sqlite write in the Go binary is still
# `// TODO` — bring it back when that lands.
[
    flatten(
        name = "static_" + arch + "_hummingbird_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@hummingbird//ca-certificates/noarch",
            "@hummingbird//mailcap/noarch",
            "@hummingbird//tzdata/noarch",
            "//oci/distroless/common:rootfs",
            "//oci/distroless/common:passwd",
            "//oci/distroless/common:home",
            "//oci/distroless/common:group",
            "//oci/distroless/common:tmp",
            "//oci/distroless/common:os_release_hummingbird",
            "//oci/distroless/common:etc_os_release_symlink",
        ],
    )
    for arch in STATIC_ARCHITECTURES["hummingbird"]
]

# rpmdb tar per arch. Aspect-driven — `targets` walks each flatten's `tars`
# and collects RpmHeaderInfo from every rpm_package reachable. No parallel
# enumeration of headers; this stays in sync with the flatten by default.
[
    rpmdb_merge(
        name = "rpmdb_" + arch + "_hummingbird",
        targets = [":static_" + arch + "_hummingbird_layer"],
    )
    for arch in STATIC_ARCHITECTURES["hummingbird"]
]

# Debug-variant rpmdb: re-uses the release layer as a target and adds the
# busybox layer on top. rpmdb_merge dedupes by (package, arch), so reusing
# the release flatten doesn't duplicate rows — and the package set never
# falls out of sync with what the release image actually ships. grype routes
# busybox CVEs through the hummingbird provider (which knows hum1 backports)
# rather than NVD-generic via the binary cataloger.
[
    rpmdb_merge(
        name = "rpmdb_debug_" + arch + "_hummingbird",
        targets = [
            ":static_" + arch + "_hummingbird_layer",
            ":busybox_" + arch + "_hummingbird_layer",
        ],
    )
    for arch in STATIC_ARCHITECTURES["hummingbird"]
]

[
    distroless_matrix(
        name = "static",
        architectures = STATIC_ARCHITECTURES[distro],
        debug_entrypoint = ["/busybox/sh"],
        debug_env = {"PATH": _DEFAULT_PATH + ":/busybox"},
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES if distro == "debian" else [],
        debug_layers = static_debug_layers,
        # Hummingbird debug == release (no busybox layer yet), so no extra VEX
        # needed beyond what `vex` already provides — passing the same doc
        # twice would trigger a `vex` duplicate-label check downstream.
        debug_vex = [":debug_vex"] if distro == "debian" else [],
        distro = distro,
        env = {
            "PATH": _DEFAULT_PATH,
            "SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
        },
        ignore_cves = _IGNORE_CVES[distro],
        layers = static_layers,
        vex = _VEX[distro],
    )
    for distro in STATIC_DISTROS
]

[
    busybox_layer(
        name = "busybox_" + arch + "_debian_layer",
        busybox = "@debian//{}/{}".format("busybox-static", arch),
        compress = COMPRESSION,
    )
    for arch in BUSYBOX_ARCHITECTURES
]

# Hummingbird busybox layers per arch. busybox is the first arch-specific
# rpm in the lockfile (everything else is noarch), so the senku arch name
# (amd64/arm64) maps to the rpm arch name (x86_64/aarch64) explicitly.
busybox_layer(
    name = "busybox_amd64_hummingbird_layer",
    busybox = "@hummingbird//busybox/x86_64",
    compress = COMPRESSION,
)

busybox_layer(
    name = "busybox_arm64_hummingbird_layer",
    busybox = "@hummingbird//busybox/aarch64",
    compress = COMPRESSION,
)

# Structure tests (amd64 only, tar driver doesn't need docker)
[
    container_structure_test(
        name = "static_" + user + "_test",
        size = "small",
        configs = [
            "test.yaml",
            "test_" + user + ".yaml",
        ],
        driver = "tar",
        image = ":static_" + user + "_amd64_debian.tar",
    )
    for user in [
        "root",
        "nonroot",
    ]
]

# Hummingbird structure tests reuse test_<user>.yaml (user/workdir/PATH are
# distro-agnostic) plus a distro-specific test_hummingbird.yaml that asserts
# the RHEL-shape paths actually shipped by ca-certificates/mailcap/tzdata
# and the senku-synthesized /etc/os-release symlink + rpmdb sqlite.
[
    container_structure_test(
        name = "static_" + user + "_hummingbird_test",
        size = "small",
        configs = [
            "test_hummingbird.yaml",
            "test_" + user + ".yaml",
        ],
        driver = "tar",
        image = ":static_" + user + "_amd64_hummingbird.tar",
    )
    for user in [
        "root",
        "nonroot",
    ]
]

container_structure_test(
    name = "static_debug_test",
    size = "small",
    configs = [
        "test.yaml",
        "test_debug.yaml",
    ],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":static_debug_nonroot_amd64_debian.oci_layout",
    tags = ["requires-docker"],
)

bzl_library(
    name = "busybox",
    srcs = ["busybox.bzl"],
    deps = [
        "//oci/distroless/common:variables",
        "@rules_distroless//distroless:defs",
    ],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
