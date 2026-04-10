load("@bazel_lib//lib:transitions.bzl", "platform_transition_filegroup")
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

PLATFORMS = {
    "linux-amd64": "@rules_go//go/toolchain:linux_amd64",
    "linux-arm64": "@rules_go//go/toolchain:linux_arm64",
    "darwin-amd64": "@rules_go//go/toolchain:darwin_amd64",
    "darwin-arm64": "@rules_go//go/toolchain:darwin_arm64",
}

[
    platform_transition_filegroup(
        name = "bifrost-" + platform,
        srcs = [":cli"],
        target_platform = label,
    )
    for platform, label in PLATFORMS.items()
]
