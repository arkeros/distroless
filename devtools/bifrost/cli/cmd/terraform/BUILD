load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "terraform",
    srcs = [
        "render.go",
        "terraform.go",
    ],
    importpath = "github.com/arkeros/senku/devtools/bifrost/cli/cmd/terraform",
    visibility = ["//devtools/bifrost/cli:__subpackages__"],
    deps = [
        "//devtools/bifrost/api",
        "//devtools/bifrost/internal",
        "//devtools/bifrost/terraform",
        "@com_github_spf13_cobra//:cobra",
    ],
)
