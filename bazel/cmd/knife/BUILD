load("@bazel_lib//lib:transitions.bzl", "platform_transition_filegroup")
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

PLATFORMS = {
    "linux-amd64": "@rules_go//go/toolchain:linux_amd64",
    "linux-arm64": "@rules_go//go/toolchain:linux_arm64",
    "darwin-amd64": "@rules_go//go/toolchain:darwin_amd64",
    "darwin-arm64": "@rules_go//go/toolchain:darwin_arm64",
}

[
    platform_transition_filegroup(
        name = "knife-" + platform,
        srcs = [":knife"],
        target_platform = label,
    )
    for platform, label in PLATFORMS.items()
]
