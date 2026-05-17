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

# Per-(version, arch) JRE tree at /jre/. Mirrors nodejs's /nodejs/ prefix —
# placed at a non-standard path so the host distro's PATH never resolves
# to it. The image entrypoint is the absolute path /jre/bin/java.
#
# We ship the *full* Temurin JRE tree (~150 MB uncompressed) rather than
# an mtree-allowlist subset. The aggressive nodejs-style "strip everything
# except bin/node" trick doesn't apply here — bin/java is a 25 KB launcher
# that dlopens lib/server/libjvm.so plus the entire lib/ tree of native
# helpers and lib/modules (the jimage). Allowlisting risks lazy-loaded
# UnsatisfiedLinkErrors that won't surface in structure tests but will
# hit production. Adoptium's JRE tarball is already trimmed (no jmods,
# no src.zip, no demo, no include) so the over-ship is bounded.
#
# Source the http_archive's filegroup directly: `mtree_spec` calls
# `to_repository_relative_path` on each File, which for an external repo
# yields paths relative to that repo's root (e.g. `bin/java`,
# `lib/server/libjvm.so`). No strip_prefix needed — `package_dir = "/jre"`
# prepends the destination prefix and we get `./jre/bin/java`.
[
    tar(
        name = "java_{}_{}_hummingbird_binary".format(major, arch),
        srcs = ["@temurin_jre_{}_{}//:all".format(major, arch)],
        mutate = mutate(
            owner = "0",
            ownername = "root",
            package_dir = "/jre",
        ),
        package_metadata = [":java_metadata_{}".format(major)],
    )
    for major in JAVA_MAJOR_VERSIONS
    for arch in JAVA_ARCHITECTURES["hummingbird"]
]

# Per-version layer = JRE tree + libatomic. Adoptium ships glibc-only JREs
# but ARM64 builds historically dlopen libatomic.so.1 for compiler-generated
# atomic ops (depends on gcc version used to build the JDK). Bundling it
# uniformly across archs keeps the rpmdb consistent and prevents a future
# Temurin minor bump from surfacing a runtime linker failure on aarch64.
# Cost is ~10KB per image. Same reasoning as the nodejs layer's libatomic.
[
    flatten(
        name = "java_{}_{}_hummingbird_layer".format(major, arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            ":java_{}_{}_hummingbird_binary".format(major, arch),
            "@hummingbird//libatomic/{}".format(HUMMINGBIRD_ARCH_MAP[arch]),
        ],
    )
    for major in JAVA_MAJOR_VERSIONS
    for arch in JAVA_ARCHITECTURES["hummingbird"]
]

# rpmdb_merge — single sqlite covering static + cc + libatomic. With no
# base inheritance, this is the only rpmdb in the release image. The JRE
# itself is *not* in the rpmdb (it's a tarball, not an rpm) — secdb
# routing for the JRE goes via the `cpe` on `java_metadata_*` above.
[
    rpmdb_merge(
        name = "rpmdb_java_{}_{}_hummingbird".format(major, arch),
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
            ":java_{}_{}_hummingbird_layer".format(major, arch),
        ],
    )
    for major in JAVA_MAJOR_VERSIONS
    for arch in JAVA_ARCHITECTURES["hummingbird"]
]

# Debug rpmdb adds busybox rows for the _debug variant.
[
    rpmdb_merge(
        name = "rpmdb_java_debug_{}_{}_hummingbird".format(major, arch),
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/static:busybox_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
            ":java_{}_{}_hummingbird_layer".format(major, arch),
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
        debug_env = DEFAULT_DEBUG_ENV,
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
            "org.opencontainers.image.description": "Distroless Java {} (Temurin JRE) image".format(major),
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
