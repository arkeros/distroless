load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "toolversions",
    srcs = ["toolversions.go"],
    importpath = "github.com/arkeros/senku/bazel/pkg/toolversions",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_bazelbuild_buildtools//build",
        "@org_golang_x_text//cases",
        "@org_golang_x_text//language",
    ],
)

go_test(
    name = "toolversions_test",
    srcs = ["toolversions_test.go"],
    embed = [":toolversions"],
    deps = ["//base/cmp"],
)
