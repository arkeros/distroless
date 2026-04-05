load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "apt",
    srcs = [
        "apt.go",
        "update.go",
        "versions.go",
    ],
    importpath = "github.com/arkeros/senku/bazel/cmd/knife/cmd/apt",
    visibility = ["//bazel/cmd/knife:__subpackages__"],
    deps = [
        "//oci/distroless/debian/lockfile",
        "//oci/distroless/debian/snapshot",
        "@com_github_spf13_cobra//:cobra",
    ],
)
