load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "grypedb",
    srcs = ["grypedb.go"],
    importpath = "github.com/arkeros/senku/bazel/pkg/grypedb",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_bazelbuild_buildtools//build",
        "@com_github_bazelbuild_buildtools//edit/bzlmod",
    ],
)

go_test(
    name = "grypedb_test",
    srcs = ["grypedb_test.go"],
    data = glob(["testdata/**"]),
    embed = [":grypedb"],
)
