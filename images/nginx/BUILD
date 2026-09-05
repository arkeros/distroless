load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@grype.bzl//grype:defs.bzl", "grype_scan")
load("@jq.bzl//jq:jq.bzl", "jq", "jq_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("@tar.bzl", "tar")
load("//images:matrix.bzl", "distroless_matrix")
load("//images:platforms.bzl", "HUMMINGBIRD_ARCH_MAP")
load("//images/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS", "NGINX_FIXED_VEX_STATEMENTS")
load("//images/common:debian_unfixed_vex.bzl", "ZLIB_UNFIXED_VEX_STATEMENTS")
load("//images/common:hummingbird_fixed_vex.bzl", "NGINX_HUMMINGBIRD_FIXED_VEX_STATEMENTS")
load("//images/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES", "DEBUG_MODE", "DEFAULT_DEBUG_ENV", "DEFAULT_ENV", "OPENSSL_WONTFIX_CVES")
load("//oci:created_timestamp.bzl", "hummingbird_created_timestamp")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document")
load(":config.bzl", "NGINX_ARCHITECTURES", "NGINX_CHANNEL_DISTROS", "NGINX_DISTROS", "NGINX_LATEST_CHANNEL", "NGINX_TAGS", "NGINX_VERSIONS", "nginx_layers")

# The lockfiles, per distro and channel: what each channel pins, and the one
# place their names are spelled. //bazel/include/oci.MODULE.bazel names the
# same files on its apt.install and rpm.install tags.
NGINX_LOCKFILES = {
    "debian": {
        "stable": "nginx-stable.lock.json",
        "mainline": "nginx-mainline.lock.json",
    },
    "hummingbird": {
        "stable": "nginx-stable.hummingbird.lock.json",
        "mainline": "nginx-mainline.hummingbird.lock.json",
    },
}

exports_files(["nginx_signing.key"] + NGINX_LOCKFILES["hummingbird"].values())

NGINX_IGNORE_CVES = {
    "stable": [],
    "mainline": [],
}

package(default_visibility = ["//visibility:public"])

# Per-channel `created` timestamps for the Hummingbird side from each
# rpm repo's `repomd.xml` revision. nginx-on-debian deliberately ships
# without `created`: nginx.org's apt repo doesn't snapshot, so there's
# no honest upstream-anchor timestamp. See #237.
[
    hummingbird_created_timestamp(
        name = "created_" + channel + "_hummingbird",
        lock = NGINX_LOCKFILES["hummingbird"][channel],
    )
    for channel in [
        "stable",
        "mainline",
    ]
]

# Per channel, because NGINX_FIXED_VEX_STATEMENTS is keyed by channel: the
# two pin different upstream versions. Both are empty today — nginx is
# identified by upstream version and CPE in the SBOM and on the image alike
# (`upstream_identities` in //bazel/include/oci.MODULE.bazel), which retired
# the distro-tracker statements and the CVE-2013-0337 log-permissions one,
# a Debian-only tracker entry that no longer reaches any scan. The documents
# stay wired so the next statement is one line.
[
    vex_document(
        name = "vex_" + channel,
        author = "rafael@arquero.cat",
        id = "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD#debian-" + channel,
        statements = GLIBC_FIXED_VEX_STATEMENTS + NGINX_FIXED_VEX_STATEMENTS[channel] + ZLIB_UNFIXED_VEX_STATEMENTS,
        timestamp = "2026-08-01T00:00:00Z",
    )
    for channel in NGINX_VERSIONS.keys()
]

[
    vex_document(
        name = "debug_vex_" + channel,
        author = "rafael@arquero.cat",
        id = "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD#debian-" + channel + "-debug",
        statements = BUSYBOX_FIXED_VEX_STATEMENTS + NGINX_FIXED_VEX_STATEMENTS[channel] + ZLIB_UNFIXED_VEX_STATEMENTS,
        timestamp = "2026-08-01T00:00:00Z",
    )
    for channel in NGINX_VERSIONS.keys()
]

# Hummingbird, per channel. Both empty today, for the same reason as the
# Debian documents above; the statements the release-tag mismatch used to
# need are recorded in //images/common:hummingbird_fixed_vex.bzl.
vex_document(
    name = "vex_hummingbird_mainline",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD#hummingbird-mainline",
    statements = [],
    timestamp = "2026-07-10T00:00:00Z",
    version = 3,
)

vex_document(
    name = "vex_hummingbird_stable",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD#hummingbird-stable",
    statements = NGINX_HUMMINGBIRD_FIXED_VEX_STATEMENTS["stable"],
    timestamp = "2026-09-05T00:00:00Z",
    version = 7,
)

_IGNORE_CVES_BY_DISTRO = {
    "debian": DEBIAN_WONTFIX_CVES + OPENSSL_WONTFIX_CVES,
    "hummingbird": [],
}

# Per (channel, distro), each document keyed by channel as its statements are.
_VEX_BY_CHANNEL_AND_DISTRO = {
    "stable": {
        "debian": [":vex_stable"],
        "hummingbird": [":vex_hummingbird_stable"],
    },
    "mainline": {
        "debian": [":vex_mainline"],
        "hummingbird": [":vex_hummingbird_mainline"],
    },
}

_NGINX_ENTRYPOINT = [
    # nginx.org ships /usr/sbin/nginx on both deb and rpm.
    "/usr/sbin/nginx",
    "-e",
    "/dev/stderr",
    "-g",
    "daemon off;",
]

tar(
    name = "nginx_conf_layer",
    srcs = [
        "default.conf",
        "nginx.conf",
    ],
    # `time=0.0` on every file entry, or bsdtar records the source file's mtime
    # — the checkout time, different on every CI runner — and the layer, the
    # image config and the index digest all change per machine. Directories
    # get the epoch without it; files do not. Found when the `release` job
    # built a digest the `publish` job had never pushed (run 33618108072).
    mtree = [
        "./etc/nginx/nginx.conf uid=0 gid=0 mode=0644 time=0.0 type=file content=$(execpath nginx.conf)",
        "./etc/nginx/conf.d type=dir uid=0 gid=0 mode=0755",
        "./etc/nginx/conf.d/default.conf uid=0 gid=0 mode=0644 time=0.0 type=file content=$(execpath default.conf)",
        "./var/www/html type=dir uid=0 gid=0 mode=0755",
    ],
)

# Debian layers (per channel × arch): nginx.org-shipped rpms via the
# distroless @nginx_stable / @nginx_mainline apt extension.
[
    flatten(
        name = version_label + "_" + arch + "_debian_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@{}//{}/{}".format(apt_repo, "nginx", arch),
        ],
    )
    for version_label, apt_repo in NGINX_VERSIONS.items()
    for arch in NGINX_ARCHITECTURES["debian"]
]

# Hummingbird-derived layers: nginx.org's RHEL10 rpms via @nginx_stable_rpm
# / @nginx_mainline_rpm. Single self-contained `nginx` rpm — same binary
# semantics as the Debian flatten on the apt side, just different package
# format. Inherits glibc/openssl/zlib/pcre2 from the cc base (no distroless-side
# soname overlays needed — nginx.org's rpm doesn't link gperftools).
_NGINX_RPM_REPO = {
    "stable": "@nginx_stable_rpm",
    "mainline": "@nginx_mainline_rpm",
}

[
    flatten(
        name = version_label + "_" + arch + "_hummingbird_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "{}//nginx/{}".format(
                _NGINX_RPM_REPO[version_label],
                HUMMINGBIRD_ARCH_MAP[arch],
            ),
        ],
    )
    for version_label in NGINX_VERSIONS.keys()
    for arch in NGINX_ARCHITECTURES["hummingbird"]
]

# rpmdb tars per (channel, arch). Merges static + cc + this nginx layer.
# Single sqlite per image — no base inheritance, no duplicate rpmdb layers.
[
    rpmdb_merge(
        name = "rpmdb_nginx_" + version_label + "_" + arch + "_hummingbird",
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
            ":" + version_label + "_" + arch + "_hummingbird_layer",
        ],
    )
    for version_label in NGINX_VERSIONS.keys()
    for arch in NGINX_ARCHITECTURES["hummingbird"]
]

# Debug rpmdbs add busybox rows.
[
    rpmdb_merge(
        name = "rpmdb_nginx_debug_" + version_label + "_" + arch + "_hummingbird",
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/static:busybox_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
            ":" + version_label + "_" + arch + "_hummingbird_layer",
        ],
    )
    for version_label in NGINX_VERSIONS.keys()
    for arch in NGINX_ARCHITECTURES["hummingbird"]
]

[
    distroless_matrix(
        name = "nginx_stable",
        architectures = NGINX_ARCHITECTURES[distro],
        created = ":created_stable_hummingbird" if distro == "hummingbird" else None,
        debug_env = DEFAULT_DEBUG_ENV,
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro],
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD",
        },
        debug_layers = nginx_layers("stable"),
        debug_vex = [":debug_vex_stable"] if distro == "debian" else [],
        distro = distro,
        entrypoint = _NGINX_ENTRYPOINT,
        env = DEFAULT_ENV,
        ignore_cves = NGINX_IGNORE_CVES["stable"] + _IGNORE_CVES_BY_DISTRO[distro],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image",
            "org.opencontainers.image.source": "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD",
        },
        layers = nginx_layers("stable"),
        vex = _VEX_BY_CHANNEL_AND_DISTRO["stable"][distro],
    )
    for distro in NGINX_CHANNEL_DISTROS["stable"]
]

[
    distroless_matrix(
        name = "nginx_mainline",
        architectures = NGINX_ARCHITECTURES[distro],
        created = ":created_mainline_hummingbird" if distro == "hummingbird" else None,
        debug_env = DEFAULT_DEBUG_ENV,
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro],
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD",
        },
        debug_layers = nginx_layers("mainline"),
        debug_vex = [":debug_vex_mainline"] if distro == "debian" else [],
        distro = distro,
        entrypoint = _NGINX_ENTRYPOINT,
        env = DEFAULT_ENV,
        ignore_cves = NGINX_IGNORE_CVES["mainline"] + _IGNORE_CVES_BY_DISTRO[distro],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image",
            "org.opencontainers.image.source": "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD",
        },
        layers = nginx_layers("mainline"),
        vex = _VEX_BY_CHANNEL_AND_DISTRO["mainline"][distro],
    )
    for distro in NGINX_CHANNEL_DISTROS["mainline"]
]

[
    mirror_push(
        name = "nginx_" + version_label + mode,
        image = ":nginx_" + version_label + mode + "_nonroot_debian",
        repository = "nginx",
        sbom = ":nginx_" + version_label + mode + "_nonroot_debian_sbom",
        tag_list = [
            version_label + ("-debug" if mode else ""),
            short_version + ("-debug" if mode else ""),
        ] + ([
            "debug" if mode else "latest",
        ] if version_label == NGINX_LATEST_CHANNEL else []),
        tags = ["manual"],
        vex = ":nginx_" + version_label + mode + "_nonroot_debian_vex",
        vulnerabilities = ":nginx_" + version_label + mode + "_nonroot_debian_vuln",
    )
    for version_label, short_version in NGINX_TAGS.items()
    for mode in DEBUG_MODE
]

[
    write_file(
        name = "nginx_" + version_label + "_expected_version",
        out = "nginx_" + version_label + "_expected_version.json",
        content = ['{"versions":["%s"]}' % short_version],
    )
    for version_label, short_version in NGINX_TAGS.items()
]

[
    jq_test(
        name = "nginx_" + version_label + "_lock_version_test",
        file1 = ":" + "nginx_" + version_label + "_expected_version",
        file2 = lockfile,
        filter2 = """{
  versions: (
    [
      .packages[]
      | select(.name == "nginx")
      | .version
      | split(".")[0:2]
      | join(".")
    ]
    | unique
  ),
}""",
    )
    for version_label, lockfile in NGINX_LOCKFILES["debian"].items()
]

# The nginx.org package must be identified in every index SBOM as
# `pkg:generic/nginx@<upstream>` with an f5:nginx CPE (`upstream_identities` on
# its apt.install and rpm.install in //bazel/include/oci.MODULE.bazel), at the
# version the lockfile pins, so a channel bump cannot leave a stale identity
# behind. Two jq readings, one of the SBOM and one of the lockfile, diffed
# per channel and distro.
# jq + diff_test rather than jq_test, whose failure-message templating breaks
# on filters with quotes and parentheses (see //oci/publish for the same
# workaround).
[
    jq(
        name = "nginx_" + version_label + "_" + distro + "_sbom_identity",
        srcs = [":nginx_" + version_label + "_nonroot_" + distro + "_sbom"],
        out = "nginx_" + version_label + "_" + distro + "_sbom_identity.json",
        filter_file = "nginx_sbom_identity.jq",
    )
    for version_label in NGINX_VERSIONS.keys()
    for distro in NGINX_CHANNEL_DISTROS[version_label]
]

[
    jq(
        name = "nginx_" + version_label + "_" + distro + "_lock_identity",
        srcs = [NGINX_LOCKFILES[distro][version_label]],
        out = "nginx_" + version_label + "_" + distro + "_lock_identity.json",
        filter_file = "nginx_lock_version.jq",
    )
    for version_label in NGINX_VERSIONS.keys()
    for distro in NGINX_CHANNEL_DISTROS[version_label]
]

[
    diff_test(
        name = "nginx_" + version_label + "_" + distro + "_sbom_identity_test",
        failure_message = "nginx is not identified as pkg:generic/nginx@<upstream> with an f5:nginx cpe in the SBOM, or its version disagrees with the lockfile. See `upstream_identities` in //bazel/include/oci.MODULE.bazel.",
        file1 = ":nginx_" + version_label + "_" + distro + "_lock_identity",
        file2 = ":nginx_" + version_label + "_" + distro + "_sbom_identity",
    )
    for version_label in NGINX_VERSIONS.keys()
    for distro in NGINX_CHANNEL_DISTROS[version_label]
]

# The image must tell a consumer's scanner the same thing the SBOM says. syft
# reads /var/lib/dpkg/status.d and the rpmdb first, and would call nginx a
# distro package at the distro's version; with no such entry for nginx it
# falls back to its nginx binary classifier, which reads the version out of
# /usr/sbin/nginx and emits pkg:generic/nginx with an nginx CPE — the same
# identity, matched against the same upstream ranges. Scan the image as a
# consumer would, in CycloneDX form so the package list is visible, and diff
# nginx's identity against the lockfile exactly as the SBOM test above does.
[
    grype_scan(
        name = "nginx_" + version_label + "_" + distro + "_image_packages",
        database = "@grype_database",
        format = "cyclonedx-json",
        image = ":nginx_" + version_label + "_nonroot_" + NGINX_ARCHITECTURES[distro][0] + "_" + distro + "_load",
    )
    for version_label in NGINX_VERSIONS.keys()
    for distro in NGINX_CHANNEL_DISTROS[version_label]
]

[
    jq(
        name = "nginx_" + version_label + "_" + distro + "_image_identity",
        srcs = [":nginx_" + version_label + "_" + distro + "_image_packages"],
        out = "nginx_" + version_label + "_" + distro + "_image_identity.json",
        filter_file = "nginx_sbom_identity.jq",
    )
    for version_label in NGINX_VERSIONS.keys()
    for distro in NGINX_CHANNEL_DISTROS[version_label]
]

[
    diff_test(
        name = "nginx_" + version_label + "_" + distro + "_image_identity_test",
        failure_message = "a scanner reading the image does not identify nginx as pkg:generic/nginx@<upstream> with an nginx CPE, or its version disagrees with the lockfile. The image must not carry a dpkg status entry or rpmdb row for nginx; see `upstream_identities` in //bazel/include/oci.MODULE.bazel.",
        file1 = ":nginx_" + version_label + "_" + distro + "_lock_identity",
        file2 = ":nginx_" + version_label + "_" + distro + "_image_identity",
    )
    for version_label in NGINX_VERSIONS.keys()
    for distro in NGINX_CHANNEL_DISTROS[version_label]
]

# Canary for the CPE route the identity above depends on. A distro purl
# fails loud, with false positives; a CPE fails quiet, with nothing found,
# and NVD's own records lacked an open-source nginx CPE for two of the three
# July 2026 CVEs — Anchore's enrichment of the grype database supplied it.
# So: scan a fixed SBOM of nginx 1.30.3, identified exactly as the images
# identify theirs, against the pinned database, and require those three
# CVEs (all fixed in 1.30.4 per https://nginx.org/en/security_advisories.html)
# to be found. The daily database bump then notices the day the route stops
# working. The actual file lists the CVEs that went missing.
grype_scan(
    name = "nginx_cpe_canary_scan",
    database = "@grype_database",
    sbom = "nginx_cpe_canary.cdx.json",
)

jq(
    name = "nginx_cpe_canary_missing",
    srcs = [":nginx_cpe_canary_scan"],
    out = "nginx_cpe_canary_missing.json",
    filter = '["CVE-2026-42533", "CVE-2026-56434", "CVE-2026-60005"] - [.matches[].vulnerability.id]',
)

write_file(
    name = "nginx_cpe_canary_expected",
    out = "nginx_cpe_canary_expected.json",
    content = [
        "[]",
        "",
    ],
)

diff_test(
    name = "nginx_cpe_canary_test",
    size = "small",
    failure_message = "grype no longer finds known nginx 1.30.3 CVEs through the f5:nginx CPE; the actual file lists the missing ones. Either the pinned database lost its open-source nginx CPE ranges or the identity in nginx_cpe_canary.cdx.json drifted from what image_sbom emits.",
    file1 = ":nginx_cpe_canary_expected",
    file2 = ":nginx_cpe_canary_missing",
)

container_structure_test(
    name = "nginx_stable_test",
    size = "small",
    configs = ["test.yaml"],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":nginx_stable_nonroot_amd64_debian.oci_layout",
    tags = ["requires-docker"],
)

container_structure_test(
    name = "nginx_mainline_test",
    size = "small",
    configs = ["test.yaml"],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":nginx_mainline_nonroot_amd64_debian.oci_layout",
    tags = ["requires-docker"],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
