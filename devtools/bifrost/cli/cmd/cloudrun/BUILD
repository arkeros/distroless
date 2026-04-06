load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "cloudrun",
    srcs = [
        "cloudrun.go",
        "render.go",
    ],
    importpath = "github.com/arkeros/senku/devtools/bifrost/cli/cmd/cloudrun",
    visibility = ["//devtools/bifrost/cli:__subpackages__"],
    deps = [
        "//devtools/bifrost/api",
        "//devtools/bifrost/cloudrun",
        "@com_github_spf13_cobra//:cobra",
    ],
)
