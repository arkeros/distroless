load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "repin_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/devtools/build/tools/tf/providers/repin",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_bazelbuild_buildtools//build",
        "@org_golang_x_mod//sumdb/dirhash",
    ],
)

go_binary(
    name = "repin",
    embed = [":repin_lib"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "repin_test",
    srcs = [
        "check_test.go",
        "main_test.go",
    ],
    data = [
        "//bazel/include:terraform.MODULE.bazel",
        "//bazel/include:terraform.providers.lock.bzl",
    ],
    embed = [":repin_lib"],
    deps = ["@com_github_bazelbuild_buildtools//build"],
)
