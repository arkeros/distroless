load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "grype",
    srcs = [
        "grype.go",
        "update.go",
    ],
    importpath = "github.com/arkeros/senku/bazel/cmd/knife/cmd/grype",
    visibility = ["//bazel/cmd/knife:__subpackages__"],
    deps = [
        "//bazel/pkg/grypedb",
        "//bazel/pkg/mod",
        "@com_github_anchore_grype//grype/db/v6/distribution",
        "@com_github_spf13_cobra//:cobra",
    ],
)
