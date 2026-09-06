load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@jq.bzl//jq:jq.bzl", "jq_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_rpm//rpm:defs.bzl", "rpmdb_merge")
load("@tar.bzl", "tar")
load("//images:matrix.bzl", "distroless_matrix")
load("//images:platforms.bzl", "HUMMINGBIRD_ARCH_MAP")
load("//images/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBUG_MODE", "DEFAULT_DEBUG_ENV", "DEFAULT_ENV")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document", "vex_statement")
load(":config.bzl", "PYTHON_ARCHITECTURES", "PYTHON_DISTROS", "PYTHON_LATEST", "PYTHON_MINOR_VERSIONS", "python_interpreter_rpm", "python_layers", "python_version")

package(default_visibility = ["//visibility:public"])

# ADR 0007 step 5: distroless python on Hummingbird, one image per line
# upstream still patches. The interpreter is Hummingbird's own rpm, built
# `--enable-shared` against the distro's libraries, so every library the
# stdlib loads — OpenSSL, SQLite, libffi, expat, xz, bzip2, mpdecimal,
# readline, ncurses, gdbm, libuuid — is a separate rpm with its own row in
# the rpmdb and its own purl in the SBOM, routed through Hummingbird's
# secdb like every other package we ship. A static build would fold those
# into libpython and a scanner would see only CPython.
#
# Not shipped, on purpose:
#   - the nis chain (libnsl2 -> libtirpc -> krb5-libs) that 3.11 and 3.12's
#     `_nis` module would load. `import nis` fails with ImportError on those
#     two lines; the module is deprecated and gone in 3.13. Shipping it would
#     add Kerberos to every image for a module nobody imports.
#   - python-pip-wheel, which `-libs` declares for ensurepip. A distroless
#     image is nobody's place to run `python -m ensurepip`.
#   - a shell. rpm's `Requires: /usr/bin/sh` on `-libs` is for a helper
#     script under ctypes/macholib that only matters on macOS.
# One document per line, so a statement lives only on the image whose scan
# it answers: `_cve_test_stale_vex` fails a document that silences nothing.
#
# CVE-2026-14534 is a flaw in fickling, a third-party pickle safety checker
# on PyPI, which Red Hat's tracker files against python3.12 and
# Hummingbird's secdb carries with no fixed version. The image ships
# CPython's stdlib only; there is no fickling in it, on any line.
_VEX_STATEMENTS = {
    "3.12": [
        vex_statement(
            expires = "2026-11-30",
            impact_statement = "CVE-2026-14534 is in fickling, a PyPI package that inspects pickle payloads, and is filed against python3.12 by the distro tracker. The image carries CPython and its standard library only; fickling is not installed.",
            justification = "vulnerable_code_not_present",
            products = ["pkg:oci/python"],
            status = "not_affected",
            vulnerability = "CVE-2026-14534",
        ),
    ],
}

[
    vex_document(
        name = "vex_hummingbird_" + minor,
        author = "rafael@arquero.cat",
        id = "https://github.com/arkeros/distroless/blob/main/images/python/BUILD#hummingbird-" + minor,
        statements = _VEX_STATEMENTS.get(minor, []),
        timestamp = "2026-09-06T00:00:00Z",
    )
    for minor in PYTHON_MINOR_VERSIONS
]

# Libraries the stdlib's extension modules link, read off `rpm -qR` of each
# `-libs` rpm (rules_rpm's pin walks the same requires into the lockfile):
#   _bz2 -> bzip2-libs         _lzma -> xz-libs          _sqlite3 -> sqlite-libs
#   _ctypes -> libffi          _decimal -> mpdecimal     _uuid -> libuuid
#   pyexpat -> expat           _gdbm/_dbm -> gdbm-libs   readline -> readline
#   _curses/_curses_panel -> ncurses-libs (libncursesw, libpanelw, libtinfo)
#   _ssl/_hashlib -> openssl-libs, and zlib -> zlib-ng-compat: both from cc.
_COMMON_LIBS = [
    "bzip2-libs",
    "expat",
    "gdbm-libs",
    "libffi",
    "libuuid",
    "mpdecimal",
    "ncurses-libs",
    "readline",
    "sqlite-libs",
    "xz-libs",
]

# Per-line extras. `_blake2` links libb2 (which links libgomp, so hashlib
# import fails without both) until 3.14 folded blake2 into the interpreter;
# `_crypt` links libxcrypt until 3.13 removed the module; `_zstd` is new in
# 3.14 and links libzstd.
_LINE_LIBS = {
    "3.11": [
        "libb2",
        "libgomp",
        "libxcrypt",
    ],
    "3.12": [
        "libb2",
        "libgomp",
        "libxcrypt",
    ],
    "3.13": [
        "libb2",
        "libgomp",
    ],
    "3.14": ["libzstd"],
}

# /usr/bin/python3 on every line, so the entrypoint and a consumer's
# `python3` do not depend on which line they pulled. Hummingbird's default
# `python3` rpm ships the symlink itself; the flat `python3.NN` rpms only
# ship the versioned binary. One entry, so it is its own layer rather
# than a reason to re-flatten the interpreter.
[
    tar(
        name = "python3_symlink_{}".format(minor),
        mtree = [
            "./usr/bin/python3 type=link link=python{}".format(minor),
        ],
    )
    for minor in PYTHON_MINOR_VERSIONS
    if python_interpreter_rpm(minor) != "python3"
]

# The libraries every line links, as one layer: the same bytes under every
# python image of an architecture, so the registry stores the blob once and
# a host that has pulled one line has this layer for the others.
[
    flatten(
        name = "python_libs_{}_hummingbird_layer".format(arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@hummingbird//{}/{}".format(
                lib,
                HUMMINGBIRD_ARCH_MAP[arch],
            )
            for lib in _COMMON_LIBS
        ],
    )
    for arch in PYTHON_ARCHITECTURES["hummingbird"]
]

# The interpreter and its stdlib, plus the libraries only this line links.
# Those ride here rather than in the shared layer above so that layer stays
# identical across lines.
[
    flatten(
        name = "python_{}_{}_hummingbird_layer".format(minor, arch),
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@hummingbird//{}/{}".format(
                python_interpreter_rpm(minor),
                HUMMINGBIRD_ARCH_MAP[arch],
            ),
            "@hummingbird//{}-libs/{}".format(
                python_interpreter_rpm(minor),
                HUMMINGBIRD_ARCH_MAP[arch],
            ),
        ] + [
            "@hummingbird//{}/{}".format(
                lib,
                HUMMINGBIRD_ARCH_MAP[arch],
            )
            for lib in _LINE_LIBS[minor]
        ],
    )
    for minor in PYTHON_MINOR_VERSIONS
    for arch in PYTHON_ARCHITECTURES["hummingbird"]
]

# rpmdb_merge — single sqlite covering static + cc + python. With no base
# inheritance, this is the only rpmdb in the release image.
[
    rpmdb_merge(
        name = "rpmdb_python_{}_{}_hummingbird".format(minor, arch),
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
            ":python_libs_{}_hummingbird_layer".format(arch),
            ":python_{}_{}_hummingbird_layer".format(minor, arch),
        ],
    )
    for minor in PYTHON_MINOR_VERSIONS
    for arch in PYTHON_ARCHITECTURES["hummingbird"]
]

# Debug rpmdb adds busybox rows for the _debug variant.
[
    rpmdb_merge(
        name = "rpmdb_python_debug_{}_{}_hummingbird".format(minor, arch),
        targets = [
            "//images/static:static_{}_hummingbird_layer".format(arch),
            "//images/static:busybox_{}_hummingbird_layer".format(arch),
            "//images/cc:cc_{}_hummingbird_layer".format(arch),
            ":python_libs_{}_hummingbird_layer".format(arch),
            ":python_{}_{}_hummingbird_layer".format(minor, arch),
        ],
    )
    for minor in PYTHON_MINOR_VERSIONS
    for arch in PYTHON_ARCHITECTURES["hummingbird"]
]

# The nis module on 3.11 and 3.12 links libnsl, whose chain (libtirpc,
# krb5) is deliberately not shipped; see the BUILD comment above. The
# elf_needs test knows, so that it can hold every other ELF to account.
_ELF_NEEDS_ALLOW = {
    minor: {
        "/usr/lib64/python{}/lib-dynload/nis.cpython-{}-*-linux-gnu.so".format(
            minor,
            minor.replace(".", ""),
        ): "nis is deprecated and gone in 3.13; its libnsl -> libtirpc -> krb5 chain is not shipped on purpose",
    }
    for minor in [
        "3.11",
        "3.12",
    ]
}

[
    distroless_matrix(
        name = "python" + minor,
        architectures = PYTHON_ARCHITECTURES[distro],
        created = "//images/common:created_" + distro,
        debug_entrypoint = ["/busybox/sh"],
        debug_env = DEFAULT_DEBUG_ENV,
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro],
        debug_layers = python_layers(minor),
        distro = distro,
        elf_needs_allow = _ELF_NEEDS_ALLOW.get(minor, {}),
        entrypoint = ["/usr/bin/python3"],
        env = DEFAULT_ENV,
        ignore_cves = [],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless Python {} image".format(minor),
            "org.opencontainers.image.source": "https://github.com/arkeros/distroless/blob/main/images/python/BUILD",
        },
        layers = python_layers(minor),
        vex = [":vex_hummingbird_" + minor],
    )
    for distro in PYTHON_DISTROS
    for minor in PYTHON_MINOR_VERSIONS
]

# Tags name the line and the release, as nginx and bash do: the release for
# pinning to it, the line for floating across it. `latest` follows the
# newest line.
[
    mirror_push(
        name = "python" + minor + mode,
        image = ":python" + minor + mode + "_nonroot_hummingbird",
        repository = "python",
        sbom = ":python" + minor + mode + "_nonroot_hummingbird_sbom",
        tag_list = [
            version + mode.replace("_", "-")
            for version in [
                minor,
                python_version(minor),
            ]
        ] + ([
            "debug" if mode else "latest",
        ] if minor == PYTHON_LATEST else []),
        tags = ["manual"],
        vex = ":python" + minor + mode + "_nonroot_hummingbird_vex",
        vulnerabilities = ":python" + minor + mode + "_nonroot_hummingbird_vuln",
    )
    for minor in PYTHON_MINOR_VERSIONS
    for mode in DEBUG_MODE
]

# The tag names a version, so it has to be the version the image ships.
# Deliberately red on a re-pin that moves a line: a tag that lies about
# what is inside is worse than a build that stops. The lockfile records
# `<upstream>-<release>.hum1`; the hyphen cut keeps the upstream part.
[
    write_file(
        name = "python_{}_expected_version".format(minor),
        out = "python_{}_expected_version.json".format(minor),
        content = ['{"versions":["%s"]}' % python_version(minor)],
    )
    for minor in PYTHON_MINOR_VERSIONS
]

[
    jq_test(
        name = "python_{}_lock_version_test".format(minor),
        file1 = ":python_{}_expected_version".format(minor),
        file2 = "//images:hummingbird.lock.json",
        filter2 = """{
  versions: (
    [
      .packages["%s"][]
      | .version
      | split("-")[0]
    ]
    | unique
  ),
}""" % python_interpreter_rpm(minor),
    )
    for minor in PYTHON_MINOR_VERSIONS
]

# One structure test per line and architecture: the shared checks, the
# architecture's dynamic-linker path, and the line's own version assertion.
# Behavioural where it counts — every extension module the library list
# above exists for is imported, so a library missing from the layer fails
# here with an ImportError rather than in production, on both arch tars.
[
    container_structure_test(
        name = "python{}_hummingbird_test".format(minor),
        size = "small",
        configs = [
            "test_amd64.yaml",
            "test_hummingbird.yaml",
            "test_python{}.yaml".format(minor),
        ],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":python{}_nonroot_amd64_hummingbird.oci_layout".format(minor),
        tags = ["requires-docker"],
    )
    for minor in PYTHON_MINOR_VERSIONS
]

# The same on the arm64 image. Tagged as java's arm64 test is: CI runs on
# amd64 and does not emulate, so it skips these; on an arm64 host they run
# natively with `--test_tag_filters=requires-native-arm64`.
[
    container_structure_test(
        name = "python{}_arm64_hummingbird_test".format(minor),
        size = "small",
        configs = [
            "test_arm64.yaml",
            "test_hummingbird.yaml",
            "test_python{}.yaml".format(minor),
        ],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":python{}_nonroot_arm64_hummingbird.oci_layout".format(minor),
        platform = "linux/arm64",
        tags = [
            "requires-docker",
            "requires-native-arm64",
        ],
    )
    for minor in PYTHON_MINOR_VERSIONS
]

# Network-bound, split out so offline runs skip it
# (`--test_tag_filters=-requires-network`), same split as cc and java.
[
    container_structure_test(
        name = "python{}_network_test".format(minor),
        size = "small",
        configs = ["test_network.yaml"],
        driver = "docker",
        exec_properties = {
            "test.workload-isolation-type": "firecracker",
            "test.init-dockerd": "true",
        },
        image = ":python{}_nonroot_amd64_hummingbird.oci_layout".format(minor),
        tags = [
            "requires-docker",
            "requires-network",
        ],
    )
    for minor in PYTHON_MINOR_VERSIONS
]

container_structure_test(
    name = "python_debug_test",
    size = "small",
    configs = ["test_debug.yaml"],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":python{}_debug_nonroot_amd64_hummingbird.oci_layout".format(PYTHON_LATEST),
    tags = ["requires-docker"],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
