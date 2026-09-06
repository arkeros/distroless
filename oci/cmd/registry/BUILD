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
    "OCI_REGISTRY",
    "OCI_REPOSITORY_PREFIX",
)
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:supply_chain.bzl", "image_supply_chain")

package(default_visibility = [
    "//oci/cmd/registry:__subpackages__",
])

go_library(
    name = "registry_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/distroless/oci/cmd/registry",
    visibility = ["//visibility:private"],
    deps = ["//oci/proxy"],
)

# Fully static: no cgo, and not PIE, since a Go PIE keeps the ELF
# interpreter even with nothing to load and the static base has no
# ld-linux. The base carried glibc by accident until images/static stopped
# shipping base-files' closure, and this binary, cgo-linked for the net
# resolver, went with it. //oci/cmd/registry:image_runs_test execs it.
go_binary(
    name = "registry",
    embed = [":registry_lib"],
    visibility = ["//visibility:public"],
)

[
    image_from_binary(
        name = "image" + mode,
        base = "//images/static:static%s_nonroot_debian" % mode,
        binary = ":registry",
        # A static binary with no runfiles: nothing but itself to ship.
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

# GHCR — the public `distroless.io` mirror surface. Signed + SLSA-attested
# via mirror_push (ADR 0006). External consumers verify with cosign.
[
    mirror_push(
        name = "image" + mode,
        image = ":image" + mode,
        repository = "registry",
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

# GAR — internal Cloud Run pull substrate. Same digest as the GHCR copy,
# different registry. IAM-trusted, NOT cosign-trusted: out of mirror_push scope
# per ADR 0006. The `deploy` job in `.github/workflows/ci.yaml` runs this, then
# resolves the digest it produced and substitutes it into `service.yaml`.
[
    image_push(
        name = "image" + mode + "_push_gar",
        image = ":image" + mode,
        registry = GAR_REGISTRY,
        repository = GAR_REPOSITORY_PREFIX + "/registry",
        # Immutable version tags are only meaningful when the workspace status
        # is stamped in.
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
