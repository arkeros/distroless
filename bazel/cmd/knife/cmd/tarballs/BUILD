load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "tarballs",
    srcs = [
        "tarballs.go",
        "update.go",
    ],
    importpath = "github.com/arkeros/distroless/bazel/cmd/knife/cmd/tarballs",
    visibility = ["//bazel/cmd/knife:__subpackages__"],
    deps = [
        "//bazel/mod",
        "//bazel/tarballs",
        "@com_github_spf13_cobra//:cobra",
    ],
)
