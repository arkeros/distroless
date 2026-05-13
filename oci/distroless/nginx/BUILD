load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@jq.bzl//jq:jq.bzl", "jq_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@tar.bzl", "tar")
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:vex.bzl", "vex_document", "vex_statement")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:debian_fixed_vex.bzl", "BUSYBOX_FIXED_VEX_STATEMENTS", "GLIBC_FIXED_VEX_STATEMENTS")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "COMPRESSION", "DEBIAN_WONTFIX_CVES", "DEBUG_MODE")
load(":config.bzl", "NGINX_ARCHITECTURES", "NGINX_DISTROS", "NGINX_LATEST_CHANNEL", "NGINX_TAGS", "NGINX_VERSIONS", "nginx_layers")

NGINX_IGNORE_CVES = {
    "stable": [],
    "mainline": [],
}

package(default_visibility = ["//visibility:public"])

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

[
    flatten(
        name = version_label + "_" + arch + "_" + distro + "_layer",
        compress = COMPRESSION,
        deduplicate = True,
        tars = [
            "@{}//{}/{}".format(apt_repo, "nginx", arch),
        ],
    )
    for version_label, apt_repo in NGINX_VERSIONS.items()
    for distro in NGINX_DISTROS
    for arch in NGINX_ARCHITECTURES[distro]
]

[
    distroless_matrix(
        name = "nginx_stable",
        architectures = NGINX_ARCHITECTURES[distro],
        base = "//oci/distroless/cc",
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES,
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        debug_vex = [":debug_vex"],
        distro = distro,
        entrypoint = [
            "/usr/sbin/nginx",
            "-e",
            "/dev/stderr",
            "-g",
            "daemon off;",
        ],
        ignore_cves = NGINX_IGNORE_CVES["stable"] + DEBIAN_WONTFIX_CVES,
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        layers = nginx_layers("stable"),
        vex = [":vex"],
    )
    for distro in NGINX_DISTROS
]

[
    distroless_matrix(
        name = "nginx_mainline",
        architectures = NGINX_ARCHITECTURES[distro],
        base = "//oci/distroless/cc",
        debug_ignore_cves = BUSYBOX_WONTFIX_CVES,
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        debug_vex = [":debug_vex"],
        distro = distro,
        entrypoint = [
            "/usr/sbin/nginx",
            "-e",
            "/dev/stderr",
            "-g",
            "daemon off;",
        ],
        ignore_cves = NGINX_IGNORE_CVES["mainline"] + DEBIAN_WONTFIX_CVES,
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        layers = nginx_layers("mainline"),
        vex = [":vex"],
    )
    for distro in NGINX_DISTROS
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
