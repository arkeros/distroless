load("@rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "knife_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/bazel/cmd/knife",
    visibility = ["//visibility:private"],
    deps = ["//bazel/cmd/knife/cmd"],
)

go_binary(
    name = "knife",
    embed = [":knife_lib"],
    visibility = ["//visibility:public"],
)
