load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_img//img:image.bzl", "image_from_binary", "image_manifest")
load("@rules_img//img:load.bzl", "image_load")
load("@rules_img//img:push.bzl", "image_push")
load("//images/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "DEBIAN_WONTFIX_CVES", "DEBUG_MODE")
load(
    "//oci:config.bzl",
    "GAR_REGISTRY",
    "GAR_REPOSITORY_PREFIX",
)
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:supply_chain.bzl", "image_supply_chain")

go_library(
    name = "server_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/distroless/web/cmd/server",
    visibility = ["//visibility:private"],
    deps = [
        "//web/internal/attestation",
        "//web/internal/compress",
        "//web/internal/directory",
        "//web/internal/mirror",
        "//web/internal/policy",
    ],
)

# Fully static, as //oci/cmd/registry:registry and for the same reason:
# the static base has no ld-linux, and a cgo or PIE Go binary asks for it.
# :image_runs_test execs it.
go_binary(
    name = "server",
    embed = [":server_lib"],
    visibility = ["//visibility:public"],
)

[
    image_from_binary(
        name = "image" + mode,
        base = "//images/static:static%s_nonroot_debian" % mode,
        binary = ":server",
        # Statically linked, and every asset it serves is `go:embed`ed —
        # nothing to carry in runfiles.
        include_runfiles = False,
        platforms = [
            "//bazel/platforms:linux_amd64",
            "//bazel/platforms:linux_arm64",
        ],
    )
    for mode in DEBUG_MODE
]

[
    image_supply_chain(
        ignore_cves = DEBIAN_WONTFIX_CVES + (BUSYBOX_WONTFIX_CVES["debian"] if mode == "_debug" else []),
        image = ":image" + mode,
    )
    for mode in DEBUG_MODE
]

[
    image_load(
        name = "image%s_load" % mode,
        image = ":image%s" % mode,
        tag_list = [
            "bazel/%s:%s" % (
                package_name(),
                "image%s" % mode,
            ),
        ],
    )
    for mode in DEBUG_MODE
]

# GHCR — published like every other mirror image, signed and attested. The
# service that renders attestations is itself attested; there is no reason for
# it to be the one exception.
[
    mirror_push(
        name = "image" + mode,
        image = ":image" + mode,
        repository = "web",
        sbom = ":image" + mode + "_sbom",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
        vulnerabilities = ":image" + mode + "_vuln",
    )
    for mode in DEBUG_MODE
]

# GAR — the Cloud Run pull substrate. Same digest as the GHCR copy, trusted by
# IAM rather than by signature, so deliberately outside mirror_push (ADR 0006).
[
    image_push(
        name = "image" + mode + "_push_gar",
        image = ":image" + mode,
        registry = GAR_REGISTRY,
        repository = GAR_REPOSITORY_PREFIX + "/web",
        stamp = "force",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
    )
    for mode in DEBUG_MODE
]

exports_files(["service.yaml"])

# The image, on the one platform docker runs here, so a test can execute
# what it ships. The index above is what deploys; this manifest is the
# same layer on the same base, pinned to amd64 for the test runner.
image_manifest(
    name = "image_amd64",
    base = "//images/static:static_nonroot_debian",
    layers = [":image.layer"],
    platform = "//bazel/platforms:linux_amd64",
    tags = ["manual"],
)

filegroup(
    name = "image_amd64.oci_layout",
    srcs = [":image_amd64"],
    output_group = "oci_layout",
    tags = ["manual"],
)

# The binary starts on the base it ships on. A file-existence test cannot
# tell a binary that runs from one whose dynamic loader is not there; this
# one execs it, and fails the way Cloud Run did before it does.
container_structure_test(
    name = "image_runs_test",
    size = "small",
    configs = ["test_runs.yaml"],
    driver = "docker",
    exec_properties = {
        "test.workload-isolation-type": "firecracker",
        "test.init-dockerd": "true",
    },
    image = ":image_amd64.oci_layout",
    tags = ["requires-docker"],
)
