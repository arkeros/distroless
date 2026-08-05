load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "webroot",
    srcs = [
        "plan.go",
        "webroot.go",
    ],
    importpath = "github.com/arkeros/senku/devtools/build/tools/webroot",
    visibility = ["//visibility:public"],
)

go_test(
    name = "webroot_test",
    srcs = [
        "plan_test.go",
        "webroot_test.go",
    ],
    deps = [":webroot"],
)
