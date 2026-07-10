load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@package_metadata//:defs.bzl", "package_metadata")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("@tar.bzl", "tar")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless:platforms.bzl", "HUMMINGBIRD_ARCH_MAP")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBUG_MODE", "DEFAULT_DEBUG_ENV", "DEFAULT_ENV")
load(":config.bzl", "NODEJS_ARCHITECTURES", "NODEJS_DISTROS", "NODEJS_MAJOR_VERSIONS", "NODEJS_VERSIONS", "nodejs_layers")

package(default_visibility = ["//visibility:public"])

# ADR 0007 step 6's "marquee demo": distroless node on Hummingbird's
# glibc-2.42-13.hum1, sourced from nodejs.org prebuilt tarballs (not from
# rpm/apt). Only `bin/node` is extracted — npm/npx/corepack/headers/docs
# stay in bazel-out and never reach the layer. That delta is what closes
# the gap to Hummingbird's 134MB hi/nodejs.
#
# Versions track Node's LTS + Current lines as of ADR drafting: 20 (maint),
# 24 (current LTS), 26 (current). Refresh by bumping the http_archive
# entries in //bazel/include/oci.MODULE.bazel together with this file.

# Hummingbird-derived node images would surface Node CVEs through syft's
# nodejs-binary cataloger -> NVD-CPE matching (Node isn't in any rpm
# secdb). No fixed-VEX statements yet — bump lockfile to clear CVEs at
# the source rather than VEXing them.
vex_document(
    name = "vex_hummingbird",
    author = "rafael@arquero.cat",
    id = "https://github.com/arkeros/senku/blob/main/oci/distroless/nodejs/BUILD#hummingbird",
    statements = [],
    timestamp = "2026-05-16T00:00:00Z",
)

# Per-version PURL — `pkg:generic/nodejs.org/node@<full_version>`. Surfaces
# node in the image's CycloneDX SBOM so the in-tree grype scan can match
# Node.js CVEs.
#
# CPE is mandatory here, not optional. Grype routes `pkg:rpm/...` and
# `pkg:deb/...` via distro secdb; `pkg:generic/...` has no built-in routing
# and only matches via an explicit `cpe` on the CycloneDX component.
# Empirically verified: 0 matches without cpe vs 50+ with it on Node 18.0.0.
# Without `cpe` this image inherits the same "fraud-by-silence" pattern
# ADR 0007 disqualified for SUSE/bci-micro:16.0 and AlmaLinux ID_LIKE remixes.
# The companion `_cve_test_silent_zero` gate in //oci:supply_chain.bzl asserts
# every SBOM component is either secdb-routable or carries a `cpe`.
[
    package_metadata(
        name = "node_metadata_{}".format(major),
        cpe = "cpe:2.3:a:nodejs:node.js:{}:*:*:*:*:*:*:*".format(full_version),
        purl = "pkg:generic/nodejs.org/node@{}".format(full_version),
    )
    for major, full_version in NODEJS_VERSIONS.items()
]

# Per-(version, arch) node binary at /nodejs/bin/node. Pattern lifted
# from nginx_conf_layer in //oci/distroless/nginx/BUILD — single-file tar
# placed at a non-standard prefix so the host distro's PATH never resolves
# to it. Image entrypoint is the absolute path /nodejs/bin/node.
[
    tar(
        name = "nodejs_{}_{}_hummingbird_binary".format(major, arch),
        srcs = ["@nodejs_{}_{}//:bin/node".format(major, arch)],
        mtree = [
            "./nodejs uid=0 gid=0 mode=0755 type=dir",
            "./nodejs/bin uid=0 gid=0 mode=0755 type=dir",
            "./nodejs/bin/node uid=0 gid=0 mode=0755 time=0.0 type=file content=$(execpath @nodejs_{}_{}//:bin/node)".format(major, arch),
        ],
        package_metadata = [":node_metadata_{}".format(major)],
    )
    for major in NODEJS_MAJOR_VERSIONS
    for arch in NODEJS_ARCHITECTURES["hummingbird"]
]

# Per-version layer = node binary + libatomic. Node 26 dynamically links
# libatomic.so.1 (verified via `strings nodejs/bin/node | grep '^lib'`);
# Node 20/24 don't, but bundle it uniformly so the rpmdb stays consistent
# across versions and a future minor bump doesn't surface a runtime
# linker failure on the structure-test image. Cost is ~10KB per image.
[
    flatten(
        name = "nodejs_{}_{}_hummingbird_layer".format(major, arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            ":nodejs_{}_{}_hummingbird_binary".format(major, arch),
            "@hummingbird//libatomic/{}".format(HUMMINGBIRD_ARCH_MAP[arch]),
        ],
    )
    for major in NODEJS_MAJOR_VERSIONS
    for arch in NODEJS_ARCHITECTURES["hummingbird"]
]

# rpmdb_merge — single sqlite covering static + cc + libatomic. With no base
# inheritance, this is the only rpmdb in the release image.
[
    rpmdb_merge(
        name = "rpmdb_nodejs_{}_{}_hummingbird".format(major, arch),
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
            ":nodejs_{}_{}_hummingbird_layer".format(major, arch),
        ],
    )
    for major in NODEJS_MAJOR_VERSIONS
    for arch in NODEJS_ARCHITECTURES["hummingbird"]
]

# Debug rpmdb adds busybox rows for the _debug variant.
[
    rpmdb_merge(
        name = "rpmdb_nodejs_debug_{}_{}_hummingbird".format(major, arch),
        targets = [
            "//oci/distroless/static:static_{}_hummingbird_layer".format(arch),
            "//oci/distroless/static:busybox_{}_hummingbird_layer".format(arch),
            "//oci/distroless/cc:cc_{}_hummingbird_layer".format(arch),
            ":nodejs_{}_{}_hummingbird_layer".format(major, arch),
        ],
    )
    for major in NODEJS_MAJOR_VERSIONS
    for arch in NODEJS_ARCHITECTURES["hummingbird"]
]

[
    distroless_matrix(
        name = "nodejs" + major,
        architectures = NODEJS_ARCHITECTURES[distro],
        # NODEJS_DISTROS is currently hummingbird-only; the Node tarball
        # version is pinned independently in MODULE.bazel and surfaces
        # via the CPE on java_metadata_<major>. Image-level `created`
        # tracks the Hummingbird base snapshot.
        created = "//oci/distroless/common:created_" + distro,
        debug_entrypoint = ["/busybox/sh"],
        debug_env = DEFAULT_DEBUG_ENV,
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro],
        debug_layers = nodejs_layers(major),
        distro = distro,
        entrypoint = ["/nodejs/bin/node"],
        env = dict(
            DEFAULT_ENV,
            NODE_ENV = "production",
        ),
        ignore_cves = [],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless Node.js {} image".format(major),
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nodejs/BUILD",
        },
        layers = nodejs_layers(major),
        vex = [":vex_hummingbird"],
    )
    for distro in NODEJS_DISTROS
    for major in NODEJS_MAJOR_VERSIONS
]

[
    mirror_push(
        name = "nodejs" + major + mode,
        image = ":nodejs" + major + mode + "_nonroot_hummingbird",
        repository = "node",
        sbom = ":nodejs" + major + mode + "_nonroot_hummingbird_sbom",
        tag_list = [
            major + ("-debug" if mode else ""),
        ],
        tags = ["manual"],
    )
    for major in NODEJS_MAJOR_VERSIONS
    for mode in DEBUG_MODE
]

[
    container_structure_test(
        name = "nodejs" + major + "_hummingbird_test",
        size = "small",
        configs = [
            "test_hummingbird.yaml",
            "test_node" + major + ".yaml",
        ],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":nodejs" + major + "_nonroot_amd64_hummingbird.oci_layout",
        tags = ["requires-docker"],
    )
    for major in NODEJS_MAJOR_VERSIONS
]

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
