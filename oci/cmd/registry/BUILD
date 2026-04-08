load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_img//img:push.bzl", "image_push")
load("//oci:config.bzl", "OCI_REGISTRY", "OCI_REPOSITORY_PREFIX")
load("//oci:go_image.bzl", "go_images_all_arch")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE")

package(default_visibility = [
    "//oci/cmd/registry:__subpackages__",
    "//oci/deploy:__pkg__",
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

[
    image_push(
        name = "image" + mode + "_push",
        image = ":image" + mode + "_nonroot",
        registry = OCI_REGISTRY,
        repository = OCI_REPOSITORY_PREFIX + "/registry",
        stamp = "enabled",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
    )
    for mode in DEBUG_MODE
]
