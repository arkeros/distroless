load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "prebuilts",
    srcs = [
        "list.go",
        "prebuilts.go",
        "update.go",
    ],
    importpath = "github.com/arkeros/senku/bazel/cmd/knife/cmd/prebuilts",
    visibility = ["//bazel/cmd/knife:__subpackages__"],
    deps = [
        "//bazel/pkg/toolversions",
        "@com_github_google_go_github_v85//github",
        "@com_github_spf13_cobra//:cobra",
    ],
)
