load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@package_metadata//:defs.bzl", "package_metadata")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("@tar.bzl", "mutate", "tar")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBUG_MODE", "DEFAULT_DEBUG_ENV", "DEFAULT_ENV")
load(":config.bzl", "HUMMINGBIRD_ARCH_MAP", "JAVA_ARCHITECTURES", "JAVA_DISTROS", "JAVA_MAJOR_VERSIONS", "JAVA_VERSIONS", "java_layers")

package(default_visibility = ["//visibility:public"])

# Headless JRE distroless on Hummingbird's glibc-2.42-13.hum1, sourced from
# Eclipse Temurin's GitHub releases (not from the Hummingbird openjdk RPM —
# Hummingbird ships 21 + 25 but not 17, and a mixed-supply-chain BUILD is
# more maintenance cost than CVE-routing benefit at the v1 stage). Mirrors
# the nodejs/BUILD shape: only the JRE tree ships at /jre/, layered onto
# the cc base; jdk binaries (javac/jlink/etc.) are never downloaded.
#
# LTS-only policy (17 / 21 / 25). Version selection + provenance rationale
# live in config.bzl. Refresh by bumping (version, build) there together
# with the http_archive entries in //bazel/include/oci.MODULE.bazel.

# Temurin tarballs are downloaded from GitHub — `pkg:generic/...` purls
# have no built-in secdb routing in grype, same caveat as nodejs. CVEs
# surface via NVD-CPE matching on the attached `cpe:2.3:a:oracle:openjdk`
# entries below. No pre-baked VEX statements; CVE response is "rebuild
# at the next Adoptium quarterly CPU" rather than chronic suppression.
vex_document(
    name = "vex_hummingbird",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/java/BUILD#hummingbird",
    statements = [],
    timestamp = "2026-05-17T00:00:00Z",
)

# Per-version PURL + CPE. Same CPE-is-mandatory reason as nodejs (see the
# long comment in //oci/distroless/nodejs/BUILD): `pkg:generic/...` does
# not route to any provider, so grype falls back to NVD-CPE matching and
# only matches if a cpe is present. NVD's OpenJDK advisories overwhelmingly
# use the `oracle:openjdk` vendor (Adoptium itself cites this vendor in
# their own security disclosures), so that's the primary CPE here. Build
# number `+<build>` is Adoptium-specific and appears only in the descriptive
# purl, not in the CPE — NVD doesn't track Adoptium build counters.
#
# The companion `_cve_test_silent_zero` gate in //oci:supply_chain.bzl
# asserts every SBOM component is either secdb-routable or carries a cpe.
[
    package_metadata(
        name = "java_metadata_{}".format(major),
        cpe = "cpe:2.3:a:oracle:openjdk:{}:*:*:*:*:*:*:*".format(version),
        purl = "pkg:generic/adoptium/temurin@{}+{}".format(version, build),
    )
    for major, (version, build) in JAVA_VERSIONS.items()
]

# Per-(version, arch) Temurin tree at /jre/. Mirrors nodejs's /nodejs/
# prefix — placed at a non-standard path so the host distro's PATH never
# resolves to it. The image entrypoint is /jre/bin/java.
#
# Two tarballs per (major, arch): JRE for release, JDK for debug. The JDK
# is a strict superset of the JRE — same `bin/java` launcher, same
# `lib/server/libjvm.so`, plus jstack/jcmd/jmap/javac etc. Debug ships
# the JDK at the *same* /jre/ path so operators don't have to special-
# case kubectl exec patterns between variants. SBOM-wise the choice
# matters: both tarballs ship the same upstream OpenJDK codebase, so
# both carry the same `:java_metadata_<major>` (one component per image,
# no double-count).
#
# We ship the full Temurin tree (~150 MB uncompressed JRE, ~330 MB JDK)
# rather than an mtree-allowlist subset. The aggressive nodejs-style
# "strip everything except bin/node" trick doesn't apply — bin/java is a
# 25 KB launcher that dlopens libjvm.so plus the entire lib/ tree of
# native helpers and lib/modules (the jimage). Allowlisting risks lazy-
# loaded UnsatisfiedLinkErrors that won't surface in structure tests but
# will hit production. Adoptium's tarballs are already trimmed (JRE has
# no jmods/src.zip/demo; JDK has no source jars) so the over-ship is
# bounded.
#
# Source the http_archive's filegroup directly: `mtree_spec` calls
# `to_repository_relative_path` on each File, which for an external repo
# yields paths relative to that repo's root (e.g. `bin/java`,
# `lib/server/libjvm.so`). No strip_prefix needed — `package_dir = "/jre"`
# prepends the destination prefix and we get `./jre/bin/java`.
[
    tar(
        name = "java_{}_{}_{}_hummingbird_binary".format(image_type, major, arch),
        srcs = ["@temurin_{}_{}_{}//:all".format(image_type, major, arch)],
        mutate = mutate(
            owner = "0",
            ownername = "root",
            package_dir = "/jre",
        ),
        package_metadata = [":java_metadata_{}".format(major)],
    )
    for image_type in [
        "jre",
        "jdk",
    ]
    for major in JAVA_MAJOR_VERSIONS
    for arch in JAVA_ARCHITECTURES["hummingbird"]
]

# Per-version layer = Temurin tree + libatomic. Adoptium's JRE/JDK are
# glibc-only, but ARM64 builds historically dlopen libatomic.so.1 for
# compiler-generated atomic ops (depends on the gcc version used to
# build the JDK). Bundling it uniformly across archs keeps the rpmdb
# consistent and prevents a future Temurin minor bump from surfacing a
# runtime linker failure on aarch64. Cost is ~10 KB per image.
[
    flatten(
        name = "java_{}_{}_{}_hummingbird_layer".format(image_type, major, arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            ":java_{}_{}_{}_hummingbird_binary".format(image_type, major, arch),
            "@hummingbird//libatomic/{}".format(HUMMINGBIRD_ARCH_MAP[arch]),
        ],
    )
    for image_type in [
        "jre",
        "jdk",
    ]
    for major in JAVA_MAJOR_VERSIONS
    for arch in JAVA_ARCHITECTURES["hummingbird"]
]

# rpmdb_merge — single sqlite covering static + cc + libatomic. With no
# base inheritance, this is the only rpmdb in the release image. The
# Temurin tree itself is *not* in the rpmdb (it's a tarball, not an rpm)
# — secdb routing for OpenJDK goes via the `cpe` on `java_metadata_*`.
# Release uses the JRE layer; debug substitutes the JDK layer (which
# differs only in additional jdk.* modules — same upstream packages).
[
    rpmdb_merge(
        name = "rpmdb_java_{}_{}_hummingbird".format(major, arch),
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
            ":java_jre_{}_{}_hummingbird_layer".format(major, arch),
        ],
    )
    for major in JAVA_MAJOR_VERSIONS
    for arch in JAVA_ARCHITECTURES["hummingbird"]
]

# Debug rpmdb adds busybox rows for the _debug variant and substitutes
# the JDK layer (jstack/jcmd/jmap/javac live here).
[
    rpmdb_merge(
        name = "rpmdb_java_debug_{}_{}_hummingbird".format(major, arch),
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/static:busybox_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
            ":java_jdk_{}_{}_hummingbird_layer".format(major, arch),
        ],
    )
    for major in JAVA_MAJOR_VERSIONS
    for arch in JAVA_ARCHITECTURES["hummingbird"]
]

[
    distroless_matrix(
        name = "java" + major,
        architectures = JAVA_ARCHITECTURES[distro],
        debug_entrypoint = ["/busybox/sh"],
        # Prepend /jre/bin so a kubectl-exec shell finds jstack/jcmd/
        # jmap/javac on PATH without operators having to type the absolute
        # path. Merged onto DEFAULT_DEBUG_ENV (which contributes /busybox).
        debug_env = {
            "PATH": "/jre/bin:" + DEFAULT_DEBUG_ENV["PATH"],
        },
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES if distro == "debian" else [],
        debug_layers = java_layers(major),
        distro = distro,
        entrypoint = [
            "/jre/bin/java",
            "-jar",
        ],
        env = DEFAULT_ENV,
        ignore_cves = [],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless Java {} (Temurin) image".format(major),
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/java/BUILD",
        },
        layers = java_layers(major),
        vex = [":vex_hummingbird"],
    )
    for distro in JAVA_DISTROS
    for major in JAVA_MAJOR_VERSIONS
]

[
    mirror_push(
        name = "java" + major + mode,
        image = ":java" + major + mode + "_nonroot_hummingbird",
        repository = "java",
        sbom = ":java" + major + mode + "_nonroot_hummingbird_sbom",
        tag_list = [
            major + ("-debug" if mode else ""),
        ],
        tags = ["manual"],
    )
    for major in JAVA_MAJOR_VERSIONS
    for mode in DEBUG_MODE
]

[
    container_structure_test(
        name = "java" + major + "_hummingbird_test",
        size = "small",
        configs = [
            "test_hummingbird.yaml",
            "test_java" + major + ".yaml",
        ],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":java" + major + "_nonroot_amd64_hummingbird.oci_layout",
        tags = ["requires-docker"],
    )
    for major in JAVA_MAJOR_VERSIONS
]

# Debug-variant structure tests — JDK at /jre/ plus busybox sh. Same
# version-assertion file works against /jre/bin/java since the JDK
# launcher reports the same OpenJDK version string as the JRE one.
[
    container_structure_test(
        name = "java" + major + "_debug_hummingbird_test",
        size = "small",
        configs = [
            "test_debug.yaml",
            "test_java" + major + ".yaml",
        ],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":java" + major + "_debug_nonroot_amd64_hummingbird.oci_layout",
        tags = ["requires-docker"],
    )
    for major in JAVA_MAJOR_VERSIONS
]

# Network-bound smoke test, split from the fast tests so offline `bazel
# test` invocations skip it (`--test_tag_filters=-requires-network`).
# Validates the live TLS stack end-to-end via keytool against Wikipedia.
[
    container_structure_test(
        name = "java" + major + "_network_hummingbird_test",
        size = "small",
        configs = ["test_network.yaml"],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":java" + major + "_nonroot_amd64_hummingbird.oci_layout",
        tags = [
            "requires-docker",
            "requires-network",
        ],
    )
    for major in JAVA_MAJOR_VERSIONS
]

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
