load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "stringslice",
    srcs = ["stringslice.go"],
    importpath = "github.com/arkeros/senku/base/flag/stringslice",
    visibility = ["//visibility:public"],
)

go_test(
    name = "stringslice_test",
    srcs = ["stringslice_test.go"],
    deps = [":stringslice"],
)
