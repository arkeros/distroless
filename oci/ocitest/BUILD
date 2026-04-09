load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "ocitest",
    testonly = True,
    srcs = ["fakeregistry.go"],
    importpath = "github.com/arkeros/senku/oci/ocitest",
    visibility = ["//oci:__subpackages__"],
    deps = [
        "@com_github_google_go_containerregistry//pkg/name",
        "@com_github_google_go_containerregistry//pkg/registry",
        "@com_github_google_go_containerregistry//pkg/v1:pkg",
        "@com_github_google_go_containerregistry//pkg/v1/random",
        "@com_github_google_go_containerregistry//pkg/v1/remote",
    ],
)
