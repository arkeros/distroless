load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "proxy",
    srcs = ["proxy.go"],
    importpath = "github.com/arkeros/senku/oci/pkg/proxy",
    visibility = ["//visibility:public"],
    deps = [
        "//base/cache/lru",
        "@com_github_google_go_containerregistry//pkg/authn",
        "@com_github_google_go_containerregistry//pkg/name",
        "@com_github_google_go_containerregistry//pkg/v1/remote/transport",
    ],
)

go_test(
    name = "proxy_test",
    srcs = ["proxy_test.go"],
    deps = [":proxy"],
)
