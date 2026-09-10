load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "bep",
    srcs = ["bep.go"],
    importpath = "github.com/arkeros/distroless/bazel/cmd/knife/cmd/bep",
    visibility = ["//visibility:public"],
    deps = [
        "//bazel/bep",
        "@com_github_spf13_cobra//:cobra",
    ],
)
