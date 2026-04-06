load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "cmd",
    srcs = ["root.go"],
    importpath = "github.com/arkeros/senku/devtools/bifrost/cli/cmd",
    visibility = ["//devtools/bifrost/cli:__pkg__"],
    deps = [
        "//devtools/bifrost/cli/cmd/cloudrun",
        "//devtools/bifrost/cli/cmd/k8s",
        "//devtools/bifrost/cli/cmd/terraform",
        "@com_github_spf13_cobra//:cobra",
    ],
)
