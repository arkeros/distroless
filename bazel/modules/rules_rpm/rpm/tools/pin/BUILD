load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "pin_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/bazel/modules/rules_rpm/rpm/tools/pin",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_sassoftware_go_rpmutils//:go-rpmutils",
    ],
)

go_binary(
    name = "pin",
    embed = [":pin_lib"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "pin_test",
    srcs = ["main_test.go"],
    embed = [":pin_lib"],
)
