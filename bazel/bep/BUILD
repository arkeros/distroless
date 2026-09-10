load("@rules_go//go:def.bzl", "go_library", "go_test")

go_test(
    name = "bep_test",
    srcs = ["bep_test.go"],
    deps = [":bep"],
)

go_library(
    name = "bep",
    srcs = ["bep.go"],
    importpath = "github.com/arkeros/distroless/bazel/bep",
    visibility = ["//visibility:public"],
)
