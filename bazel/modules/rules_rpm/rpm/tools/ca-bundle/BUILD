load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "ca-bundle_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/distroless/bazel/modules/rules_rpm/rpm/tools/ca-bundle",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "ca-bundle",
    embed = [":ca-bundle_lib"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "ca-bundle_test",
    srcs = ["main_test.go"],
    embed = [":ca-bundle_lib"],
)
