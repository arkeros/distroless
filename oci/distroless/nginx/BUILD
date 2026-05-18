load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@jq.bzl//jq:jq.bzl", "jq_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("@tar.bzl", "tar")
load("//oci:created_timestamp.bzl", "hummingbird_created_timestamp")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document", "vex_statement")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless:platforms.bzl", "HUMMINGBIRD_ARCH_MAP")
load("//oci/distroless/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES", "DEBUG_MODE", "DEFAULT_DEBUG_ENV", "DEFAULT_ENV")
load(":config.bzl", "NGINX_ARCHITECTURES", "NGINX_CHANNEL_DISTROS", "NGINX_DISTROS", "NGINX_LATEST_CHANNEL", "NGINX_TAGS", "NGINX_VERSIONS", "nginx_layers")

exports_files([
    "nginx_signing.key",
    "nginx-stable.hummingbird.lock.json",
    "nginx-mainline.hummingbird.lock.json",
])

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
        lock = ":nginx-" + channel + ".hummingbird.lock.json",
    )
    for channel in [
        "stable",
        "mainline",
    ]
]

# CVE-2013-0337 — nginx default config historically created
# access.log/error.log with world-readable permissions, exposing log
# contents to local users on shared hosts. Debian's tracker marks it
# `<unfixed>` in sid and `<ignored>` "Minor issue" in trixie/bookworm/
# bullseye, blocked on upstream nginx#376. Our image never creates
# log files: nginx.conf:3 sets `error_log /dev/stderr`, nginx.conf:20
# sets `access_log /dev/stdout`, and the entrypoint additionally pins
# `-e /dev/stderr` so the error-log destination is fixed at the CLI
# level regardless of any conf override. The file-creation code path
# the CVE describes is never reached. Drops the moment Debian or
# upstream lands a fix (`_cve_test_stale_vex` enforces).
NGINX_VEX_STATEMENTS = [
    vex_statement(
        expires = "2026-08-11",
        impact_statement = "nginx.conf routes error_log to /dev/stderr and access_log to /dev/stdout; entrypoint pins -e /dev/stderr. No log files are created on disk, so the world-readable log-file default is never reached.",
        justification = "vulnerable_code_not_in_execute_path",
        products = ["pkg:oci/nginx"],
        status = "not_affected",
        vulnerability = "CVE-2013-0337",
    ),
]

vex_document(
    name = "vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
    statements = GLIBC_FIXED_VEX_STATEMENTS + NGINX_VEX_STATEMENTS,
    timestamp = "2026-05-13T00:00:00Z",
)

vex_document(
    name = "debug_vex",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD#debug",
    statements = BUSYBOX_FIXED_VEX_STATEMENTS + NGINX_VEX_STATEMENTS,
    timestamp = "2026-05-13T00:00:00Z",
)

# Stable nginx on hummingbird: nginx 1.30.1-1.el10.ngx is upstream
# nginx 1.30.1 (the version NVD identifies as the fix for
# CVE-2026-42945). Hummingbird's secdb advisory is release-suffix-
# strict (`< 1.30.1-1.hum1`), so rpmvercmp considers `el10.ngx < hum1`
# and grype flags our package — even though the rewrite-module fix is
# in upstream 1.30.1 shared by both release tags. Drops when either
# grype stops release-suffix-strict matching across rpm vendors or
# Hummingbird relaxes their advisory to `< 1.30.1` (upstream-correct
# shape). `_cve_test_stale_vex` enforces.
NGINX_HUMMINGBIRD_STABLE_VEX_STATEMENTS = [
    vex_statement(
        expires = "2026-08-14",
        impact_statement = "nginx 1.30.1-1.el10.ngx is upstream nginx 1.30.1, the version NVD identifies as the fix. Hummingbird's release-suffix-strict `< 1.30.1-1.hum1` constraint flags the rpm because `el10.ngx < hum1` in rpmvercmp, but the rewrite-module vulnerable code path is patched in the shared upstream source.",
        justification = "vulnerable_code_not_present",
        products = ["pkg:oci/nginx"],
        status = "not_affected",
        vulnerability = "CVE-2026-42945",
    ),
]

# Mainline nginx on hummingbird (1.31.0+) is rpmvercmp-above all
# current Hummingbird nginx advisories — no false positives surface,
# nothing to VEX. Empty document keeps the matrix wiring uniform.
vex_document(
    name = "vex_hummingbird_mainline",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD#hummingbird-mainline",
    statements = [],
    timestamp = "2026-05-16T00:00:00Z",
)

vex_document(
    name = "vex_hummingbird_stable",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD#hummingbird-stable",
    statements = NGINX_HUMMINGBIRD_STABLE_VEX_STATEMENTS,
    timestamp = "2026-05-16T00:00:00Z",
)

_IGNORE_CVES_BY_DISTRO = {
    "debian": DEBIAN_WONTFIX_CVES,
    "hummingbird": [],
}

# Per (channel, distro). Hummingbird stable carries the CVE-2026-42945
# release-suffix-strict false-positive VEX; mainline doesn't surface
# that CVE because 1.31.0 is rpmvercmp-above the constraint. Debian
# uses the same VEX doc for both channels — the CVE-2013-0337
# log-permissions VEX applies regardless.
_VEX_BY_CHANNEL_AND_DISTRO = {
    "stable": {
        "debian": [":vex"],
        "hummingbird": [":vex_hummingbird_stable"],
    },
    "mainline": {
        "debian": [":vex"],
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
    mtree = [
        "./etc/nginx/nginx.conf uid=0 gid=0 mode=0644 type=file content=$(execpath nginx.conf)",
        "./etc/nginx/conf.d type=dir uid=0 gid=0 mode=0755",
        "./etc/nginx/conf.d/default.conf uid=0 gid=0 mode=0644 type=file content=$(execpath default.conf)",
        "./var/www/html type=dir uid=0 gid=0 mode=0755",
    ],
)

# Debian layers (per channel × arch): nginx.org-shipped rpms via the
# senku @nginx_stable / @nginx_mainline apt extension.
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
# format. Inherits glibc/openssl/zlib/pcre2 from the cc base (no senku-side
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
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
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
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/static:busybox_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
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
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES if distro == "debian" else [],
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        debug_layers = nginx_layers("stable"),
        debug_vex = [":debug_vex"] if distro == "debian" else [],
        distro = distro,
        entrypoint = _NGINX_ENTRYPOINT,
        env = DEFAULT_ENV,
        ignore_cves = NGINX_IGNORE_CVES["stable"] + _IGNORE_CVES_BY_DISTRO[distro],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
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
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES if distro == "debian" else [],
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        debug_layers = nginx_layers("mainline"),
        debug_vex = [":debug_vex"] if distro == "debian" else [],
        distro = distro,
        entrypoint = _NGINX_ENTRYPOINT,
        env = DEFAULT_ENV,
        ignore_cves = NGINX_IGNORE_CVES["mainline"] + _IGNORE_CVES_BY_DISTRO[distro],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
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
    for version_label, lockfile in {
        "stable": "nginx-stable.lock.json",
        "mainline": "nginx-mainline.lock.json",
    }.items()
]

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
