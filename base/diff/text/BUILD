load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "text",
    srcs = [
        "print.go",
        "text.go",
    ],
    importpath = "github.com/arkeros/senku/base/diff/text",
    visibility = ["//visibility:public"],
    deps = ["//base/diff"],
)

go_test(
    name = "text_test",
    srcs = ["text_test.go"],
    embed = [":text"],
)
