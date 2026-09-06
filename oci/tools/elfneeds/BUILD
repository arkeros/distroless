load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "elfneeds_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/distroless/oci/tools/elfneeds",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "elfneeds",
    embed = [":elfneeds_lib"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "elfneeds_test",
    srcs = ["main_test.go"],
    embed = [":elfneeds_lib"],
)
