load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "proxy",
    srcs = ["proxy.go"],
    importpath = "github.com/arkeros/distroless/oci/pkg/proxy",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_google_go_containerregistry//pkg/authn",
        "@com_github_google_go_containerregistry//pkg/name",
        "@com_github_google_go_containerregistry//pkg/v1/remote/transport",
        "@com_github_hashicorp_golang_lru_v2//:golang-lru",
    ],
)

go_test(
    name = "proxy_test",
    srcs = ["proxy_test.go"],
    deps = [
        ":proxy",
        "//oci/ocitest",
        "@com_github_google_go_containerregistry//pkg/v1:pkg",
    ],
)
