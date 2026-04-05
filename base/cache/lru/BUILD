load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "lru",
    srcs = ["lru.go"],
    importpath = "github.com/arkeros/senku/base/cache/lru",
    visibility = ["//visibility:public"],
)

go_test(
    name = "lru_test",
    srcs = ["lru_test.go"],
    deps = [":lru"],
)
