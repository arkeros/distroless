load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "snapshots",
    srcs = [
        "snapshots.go",
        "update.go",
    ],
    importpath = "github.com/arkeros/senku/bazel/cmd/knife/cmd/snapshots",
    visibility = ["//bazel/cmd/knife:__subpackages__"],
    deps = [
        "//oci/distroless/debian/snapshot",
        "@com_github_spf13_cobra//:cobra",
    ],
)
