load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "k8s",
    srcs = [
        "k8s.go",
        "render.go",
    ],
    importpath = "github.com/arkeros/senku/devtools/bifrost/cli/cmd/k8s",
    visibility = ["//devtools/bifrost/cli:__subpackages__"],
    deps = [
        "//devtools/bifrost/api",
        "//devtools/bifrost/k8s",
        "@com_github_spf13_cobra//:cobra",
    ],
)
