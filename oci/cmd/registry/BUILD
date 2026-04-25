load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_img//img:push.bzl", "image_push")
load("//devtools/build/tools/image_tfvars:image_tfvars.bzl", "image_tfvars")
load("//oci:config.bzl", "GAR_REGISTRY", "GAR_REPOSITORY_PREFIX", "OCI_REGISTRY", "OCI_REPOSITORY_PREFIX")
load("//oci:go_image.bzl", "go_images_all_arch")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE")

package(default_visibility = [
    "//oci/cmd/registry:__subpackages__",
])

go_library(
    name = "registry_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/oci/cmd/registry",
    visibility = ["//visibility:private"],
    deps = [
        "//base/flag/stringslice",
        "//oci/pkg/proxy",
    ],
)

go_binary(
    name = "registry",
    embed = [":registry_lib"],
    visibility = ["//visibility:public"],
)

go_images_all_arch(
    name = "image",
    binary = ":registry",
    distro = "debian13",
    static = True,
)

# Two push destinations per mode: GHCR for the public `distroless.io` mirror,
# GAR for the Cloud Run deploy path (Cloud Run can't pull from GHCR directly).
# Same image, same digest, two hosts — pushed independently.
[
    image_push(
        name = "image" + mode + "_push_" + dest_suffix,
        image = ":image" + mode + "_nonroot",
        registry = registry,
        repository = repository_prefix + "/registry",
        stamp = "enabled",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
    )
    for mode in DEBUG_MODE
    for dest_suffix, registry, repository_prefix in [
        ("ghcr", OCI_REGISTRY, OCI_REPOSITORY_PREFIX),
        ("gar", GAR_REGISTRY, GAR_REPOSITORY_PREFIX),
    ]
]

# The digest of the just-built image, pinned into the Terraform root as a
# gitignored `image.auto.tfvars.json`. Running `:image_tfvars` materializes
# the file; `terraform apply` auto-loads it and deploys the exact digest
# Bazel just built.
image_tfvars(
    name = "image_tfvars",
    image_push = ":image_push_gar",
    out_file = "terraform/image.auto.tfvars.json",
)
