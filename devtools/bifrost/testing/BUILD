load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "testing",
    testonly = True,
    srcs = ["testing.go"],
    importpath = "github.com/arkeros/senku/devtools/bifrost/testing",
    visibility = ["//visibility:public"],
    deps = ["//devtools/bifrost/api"],
)
