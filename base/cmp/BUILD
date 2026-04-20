load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "cmp",
    srcs = ["cmp.go"],
    importpath = "github.com/arkeros/senku/base/cmp",
    visibility = ["//visibility:public"],
)

go_test(
    name = "cmp_test",
    srcs = ["cmp_test.go"],
    embed = [":cmp"],
)
