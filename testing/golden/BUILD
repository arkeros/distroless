load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "golden",
    testonly = True,
    srcs = ["golden.go"],
    importpath = "github.com/arkeros/senku/testing/golden",
    visibility = ["//visibility:public"],
    deps = ["//base/diff/text"],
)
