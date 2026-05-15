load("@rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "pin_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/bazel/modules/rules_rpm/rpm/tools/pin",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "pin",
    embed = [":pin_lib"],
    visibility = ["//visibility:public"],
)
