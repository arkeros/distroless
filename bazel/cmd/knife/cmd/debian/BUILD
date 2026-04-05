load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "debian",
    srcs = [
        "debian.go",
        "versions.go",
    ],
    importpath = "github.com/arkeros/senku/bazel/cmd/knife/cmd/debian",
    visibility = ["//bazel/cmd/knife:__subpackages__"],
    deps = [
        "//oci/distroless/debian/lockfile",
        "@com_github_spf13_cobra//:cobra",
    ],
)
