load("@rules_go//go:def.bzl", "go_library", "go_test")

go_test(
    name = "copyright_test",
    srcs = ["copyright_test.go"],
    deps = [":copyright"],
)

go_library(
    name = "copyright",
    srcs = ["copyright.go"],
    importpath = "github.com/arkeros/distroless/oci/debian/copyright",
    visibility = ["//visibility:public"],
)
