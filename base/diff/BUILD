load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "diff",
    srcs = ["diff.go"],
    importpath = "github.com/arkeros/senku/base/diff",
    visibility = ["//visibility:public"],
)

go_test(
    name = "diff_test",
    srcs = ["diff_test.go"],
    embed = [":diff"],
)
