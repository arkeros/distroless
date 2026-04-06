load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "cli_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/devtools/bifrost/cli",
    visibility = ["//visibility:private"],
    deps = [
        "//devtools/bifrost/cli/cmd",
    ],
)

go_binary(
    name = "cli",
    embed = [":cli_lib"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "cli_test",
    srcs = ["bifrost_test.go"],
    data = glob(["testdata/**"]),
    embed = [":cli_lib"],
    deps = ["//devtools/bifrost/api"],
)
