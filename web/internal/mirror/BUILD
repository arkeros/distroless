load("@rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//web:__subpackages__"])

go_library(
    name = "mirror",
    srcs = ["mirror.go"],
    importpath = "github.com/arkeros/distroless/web/internal/mirror",
    deps = [
        "//web/internal/attestation",
        "//web/internal/directory",
        "@com_github_cyclonedx_cyclonedx_go//:cyclonedx-go",
        "@com_github_google_go_containerregistry//pkg/name",
        "@com_github_google_go_containerregistry//pkg/v1:pkg",
        "@com_github_google_go_containerregistry//pkg/v1/remote",
        "@com_github_hashicorp_golang_lru_v2//:golang-lru",
    ],
)

go_test(
    name = "mirror_test",
    srcs = ["mirror_test.go"],
    deps = [
        ":mirror",
        "//oci/ocitest",
        "//web/internal/attestation",
        "@com_github_google_go_containerregistry//pkg/name",
        "@com_github_google_go_containerregistry//pkg/v1:pkg",
        "@com_github_google_go_containerregistry//pkg/v1/empty",
        "@com_github_google_go_containerregistry//pkg/v1/mutate",
        "@com_github_google_go_containerregistry//pkg/v1/random",
        "@com_github_google_go_containerregistry//pkg/v1/remote",
        "@com_github_google_go_containerregistry//pkg/v1/static",
        "@com_github_google_go_containerregistry//pkg/v1/types",
    ],
)
