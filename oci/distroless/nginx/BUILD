load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load("@rules_img//img:push.bzl", "image_push")
load("@tar.bzl", "tar")
load("//oci:config.bzl", "OCI_REGISTRY", "OCI_REPOSITORY_PREFIX")
load("//oci/distroless:matrix.bzl", "distroless_matrix")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE")
load(":config.bzl", "NGINX_ARCHITECTURES", "NGINX_DISTROS", "NGINX_TAGS", "NGINX_VERSIONS", "nginx_layers")

# CVE-2022-4899: libzstd, disputed - fixed in 2022 (https://github.com/facebook/zstd/issues/3200)
# CVE-2026-1642: nginx HTTP/2 HEADERS frame DoS, stable only - fixed in mainline 1.29.5+ (https://nvd.nist.gov/vuln/detail/CVE-2026-1642)
NGINX_IGNORE_CVES = {
    "stable": [
        "CVE-2022-4899",
        "CVE-2026-1642",
    ],
    "mainline": [
        "CVE-2022-4899",
    ],
}

package(default_visibility = ["//visibility:public"])

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
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        distro = distro,
        entrypoint = [
            "/usr/sbin/nginx",
            "-e",
            "/dev/stderr",
            "-g",
            "daemon off;",
        ],
        ignore_cves = NGINX_IGNORE_CVES["stable"],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx stable image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        layers = nginx_layers("stable"),
    )
    for distro in NGINX_DISTROS
]

[
    distroless_matrix(
        name = "nginx_mainline",
        architectures = NGINX_ARCHITECTURES[distro],
        base = "//oci/distroless/cc",
        debug_index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image (debug)",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        distro = distro,
        entrypoint = [
            "/usr/sbin/nginx",
            "-e",
            "/dev/stderr",
            "-g",
            "daemon off;",
        ],
        ignore_cves = NGINX_IGNORE_CVES["mainline"],
        index_annotations = {
            "org.opencontainers.image.description": "Distroless nginx mainline image",
            "org.opencontainers.image.source": "https://github.com/arkeros/senku/blob/main/oci/distroless/nginx/BUILD",
        },
        layers = nginx_layers("mainline"),
    )
    for distro in NGINX_DISTROS
]

[
    image_push(
        name = "nginx_" + version_label + mode + "_push",
        image = ":nginx_" + version_label + mode + "_nonroot_debian13",
        registry = OCI_REGISTRY,
        repository = OCI_REPOSITORY_PREFIX + "/nginx",
        stamp = "enabled",
        tag_list = [
            version_label + ("-debug" if mode else ""),
            short_version + ("-debug" if mode else ""),
        ] + ([
            "debug" if mode else "latest",
        ] if version_label == "mainline" else []),
        tags = ["manual"],
    )
    for version_label, short_version in NGINX_TAGS.items()
    for mode in DEBUG_MODE
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
    image = ":nginx_stable_nonroot_amd64_debian13.oci_layout",
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
    image = ":nginx_mainline_nonroot_amd64_debian13.oci_layout",
    tags = ["requires-docker"],
)

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
