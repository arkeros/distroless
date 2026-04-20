load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "cmd",
    srcs = ["root.go"],
    importpath = "github.com/arkeros/senku/bazel/cmd/knife/cmd",
    visibility = ["//bazel/cmd/knife:__pkg__"],
    deps = [
        "//bazel/cmd/knife/cmd/apt",
        "//bazel/cmd/knife/cmd/grype",
        "//bazel/cmd/knife/cmd/prebuilts",
        "@com_github_spf13_cobra//:cobra",
    ],
)
