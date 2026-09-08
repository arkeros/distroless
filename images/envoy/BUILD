load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@grype.bzl//grype:defs.bzl", "grype_scan")
load("@jq.bzl//jq:jq.bzl", "jq")
load("@package_metadata//:defs.bzl", "package_metadata")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("@tar.bzl", "tar")
load("//images:matrix.bzl", "distroless_matrix")
load("//images/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBUG_MODE", "DEFAULT_DEBUG_ENV", "DEFAULT_ENV")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document")
load(":config.bzl", "ENVOY_ARCHITECTURES", "ENVOY_DISTROS", "ENVOY_LATEST", "ENVOY_LINES", "ENVOY_VERSIONS", "envoy_layers", "envoy_repo_line")

package(default_visibility = ["//visibility:public"])

exports_files(["envoy.lock.json"])

# Distroless envoy on Hummingbird's glibc, from the bare binaries
# envoyproxy attaches to its GitHub releases (ADR 0007 step 6, the pattern
# //images/nodejs established). Upstream's own image is Ubuntu with a shell,
# a package manager and an entrypoint script wrapped around one
# internet-facing C++ parser; this is the same binary with none of that.
#
# Envoy links glibc and nothing else — no libstdc++, and BoringSSL is
# static — so the cc base covers it. Four lines, the ones upstream still
# patches; the pins live in envoy.lock.json, see config.bzl.

# Every line, as (dotted line, repo-name line, version). Target names carry
# the dot — `envoy1.39`, as //images/python names `python3.11` — but the
# tarball repos cannot, so the layers below are keyed by the stripped form.
_LINES = [
    (
        line,
        envoy_repo_line(line),
        version,
    )
    for line, version in ENVOY_VERSIONS.items()
]

# No fixed-VEX statements yet — bump the lockfile to clear CVEs at the
# source rather than VEXing them. Wired so the first statement is one line.
vex_document(
    name = "vex_hummingbird",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/distroless/blob/main/images/envoy/BUILD#hummingbird",
    statements = [],
    timestamp = "2026-09-08T00:00:00Z",
)

# Per-line identity. The purl has no namespace on purpose: it must be
# byte-identical to what syft's envoy-binary classifier emits when a
# consumer scans the image, or `envoy_<line>_image_identity_test` below
# fails. The CPE is what routes a `pkg:generic/...` component to NVD at
# all — see the same reasoning at length in //images/nodejs:BUILD.
[
    package_metadata(
        name = "envoy_metadata_{}".format(short),
        cpe = "cpe:2.3:a:envoyproxy:envoy:{}:*:*:*:*:*:*:*".format(version),
        purl = "pkg:generic/envoy@{}".format(version),
    )
    for _, short, version in _LINES
]

# Per-(line, arch) binary at /usr/bin/envoy — upstream's own path, and the
# one syft's `**/envoy` classifier glob needs to find it. `time=0.0` on the
# file entry, or bsdtar records the checkout time and every digest below it
# differs per machine (see the note in //images/nginx:BUILD).
[
    tar(
        name = "envoy_{}_{}_hummingbird_binary".format(short, arch),
        srcs = ["@envoy_{}_{}//:envoy".format(short, arch)],
        mtree = [
            "./usr/bin/envoy uid=0 gid=0 mode=0755 time=0.0 type=file content=$(execpath @envoy_{}_{}//:envoy)".format(short, arch),
        ],
        package_metadata = [":envoy_metadata_{}".format(short)],
    )
    for _, short, _version in _LINES
    for arch in ENVOY_ARCHITECTURES["hummingbird"]
]

[
    flatten(
        name = "envoy_{}_{}_hummingbird_layer".format(short, arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [":envoy_{}_{}_hummingbird_binary".format(short, arch)],
    )
    for _, short, _version in _LINES
    for arch in ENVOY_ARCHITECTURES["hummingbird"]
]

# rpmdb_merge — single sqlite covering static + cc. With no base
# inheritance, this is the only rpmdb in the release image; the envoy layer
# carries no rpm of its own, which is exactly why the identity tests below
# exist.
[
    rpmdb_merge(
        name = "rpmdb_envoy_{}_{}_hummingbird".format(short, arch),
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
        ],
    )
    for _, short, _version in _LINES
    for arch in ENVOY_ARCHITECTURES["hummingbird"]
]

# Debug rpmdb adds busybox rows for the _debug variant.
[
    rpmdb_merge(
        name = "rpmdb_envoy_debug_{}_{}_hummingbird".format(short, arch),
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/static:busybox_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
        ],
    )
    for _, short, _version in _LINES
    for arch in ENVOY_ARCHITECTURES["hummingbird"]
]

# No /etc/envoy/envoy.yaml. Upstream's deb ships a demo config that proxies
# to www.envoyproxy.io; a base image should not have an opinion about where
# traffic goes. Without `-c` or `--config-yaml`, envoy exits with its own
# usage error, which says what is missing.
[
    distroless_matrix(
        name = "envoy" + line,
        architectures = ENVOY_ARCHITECTURES[distro],
        # envoy is identified by its upstream, on the image as in the SBOM;
        # the consumer's scan is what proves the image agrees. ADR 0016.
        consumer_scan = True,
        # The envoy version is pinned independently in envoy.lock.json and
        # surfaces via the CPE on envoy_metadata_<line>. Image-level
        # `created` tracks the Hummingbird base snapshot.
        created = "//images/common:created_" + distro,
        debug_entrypoint = ["/busybox/sh"],
        debug_env = DEFAULT_DEBUG_ENV,
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro],
        debug_layers = envoy_layers(line),
        distro = distro,
        entrypoint = ["/usr/bin/envoy"],
        env = DEFAULT_ENV,
        ignore_cves = [],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless Envoy {} image".format(line),
            "org.opencontainers.image.source": "https://github.com/arkeros/distroless/blob/main/images/envoy/BUILD",
        },
        layers = envoy_layers(line),
        vex = [":vex_hummingbird"],
    )
    for distro in ENVOY_DISTROS
    for line in ENVOY_LINES
]

[
    mirror_push(
        name = "envoy" + line + mode,
        image = ":envoy" + line + mode + "_nonroot_hummingbird",
        repository = "envoy",
        sbom = ":envoy" + line + mode + "_nonroot_hummingbird_sbom",
        tag_list = [
            line + ("-debug" if mode else ""),
        ] + ([
            "debug" if mode else "latest",
        ] if line == ENVOY_LATEST else []),
        tags = ["manual"],
        vex = ":envoy" + line + mode + "_nonroot_hummingbird_vex",
        vulnerabilities = ":envoy" + line + mode + "_nonroot_hummingbird_vuln",
    )
    for line in ENVOY_LINES
    for mode in DEBUG_MODE
]

# envoy must be identified in every index SBOM as `pkg:generic/envoy@<v>`
# with an envoyproxy:envoy CPE, at the version the lockfile pins, so a line
# bump cannot leave a stale identity behind. Two jq readings, one of the
# SBOM and one of the lockfile, diffed per line.
#
# jq + diff_test rather than jq_test, whose failure-message templating
# breaks on filters with quotes and parentheses (see //images/nginx:BUILD
# for the same workaround).
[
    jq(
        name = "envoy_" + line + "_sbom_identity",
        srcs = [":envoy" + line + "_nonroot_hummingbird_sbom"],
        out = "envoy_" + line + "_sbom_identity.json",
        filter_file = "envoy_sbom_identity.jq",
    )
    for line in ENVOY_LINES
]

[
    jq(
        name = "envoy_" + line + "_lock_identity",
        srcs = ["envoy.lock.json"],
        out = "envoy_" + line + "_lock_identity.json",
        args = [
            "--arg",
            "line",
            line,
        ],
        filter_file = "envoy_lock_version.jq",
    )
    for line in ENVOY_LINES
]

[
    diff_test(
        name = "envoy_" + line + "_sbom_identity_test",
        failure_message = "envoy is not identified as pkg:generic/envoy@<upstream> with an envoyproxy:envoy cpe in the SBOM, or its version disagrees with envoy.lock.json. See envoy_metadata_<line> in this file.",
        file1 = ":envoy_" + line + "_lock_identity",
        file2 = ":envoy_" + line + "_sbom_identity",
    )
    for line in ENVOY_LINES
]

# The image must tell a consumer's scanner the same thing the SBOM says.
# syft reads the rpmdb first and finds no envoy row there — the binary comes
# from a release asset, not a package — so it falls back to its envoy-binary
# classifier, which reads the version out of /usr/bin/envoy and emits
# pkg:generic/envoy with an envoyproxy CPE: the same identity, matched
# against the same ranges. Scan the image as a consumer would, in CycloneDX
# form so the package list is visible, and diff envoy's identity against the
# lockfile exactly as the SBOM test above does.
#
# This is also the test that will notice when the classifier goes silent:
# syft matches envoy's version banner with `1\.3[0-9]\.[0-9]+`, which stops
# matching the day upstream cuts 1.40.
[
    grype_scan(
        name = "envoy_" + line + "_image_packages",
        database = "@grype_database",
        format = "cyclonedx-json",
        image = ":envoy" + line + "_nonroot_" + ENVOY_ARCHITECTURES["hummingbird"][0] + "_hummingbird_load",
    )
    for line in ENVOY_LINES
]

[
    jq(
        name = "envoy_" + line + "_image_identity",
        srcs = [":envoy_" + line + "_image_packages"],
        out = "envoy_" + line + "_image_identity.json",
        filter_file = "envoy_sbom_identity.jq",
    )
    for line in ENVOY_LINES
]

[
    diff_test(
        name = "envoy_" + line + "_image_identity_test",
        failure_message = "a scanner reading the image does not identify envoy as pkg:generic/envoy@<upstream> with an envoyproxy:envoy CPE, or its version disagrees with envoy.lock.json. syft's envoy-binary classifier matches the version banner with `1\\.3[0-9]\\.[0-9]+`, so this goes red when upstream cuts 1.40.",
        file1 = ":envoy_" + line + "_lock_identity",
        file2 = ":envoy_" + line + "_image_identity",
    )
    for line in ENVOY_LINES
]

[
    container_structure_test(
        name = "envoy" + line + "_hummingbird_test",
        size = "small",
        configs = [
            "test_hummingbird.yaml",
            "test_envoy" + line + ".yaml",
        ],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":envoy" + line + "_nonroot_amd64_hummingbird.oci_layout",
        tags = ["requires-docker"],
    )
    for line in ENVOY_LINES
]

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
    deps = ["@envoy_tarballs//:versions"],
)
