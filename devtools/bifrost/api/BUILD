load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "api",
    srcs = [
        "environment.go",
        "types.go",
    ],
    importpath = "github.com/arkeros/senku/devtools/bifrost/api",
    visibility = ["//visibility:public"],
    deps = [
        "@io_k8s_api//core/v1:core",
        "@io_k8s_apimachinery//pkg/api/resource",
        "@io_k8s_sigs_yaml//:yaml",
    ],
)

go_test(
    name = "api_test",
    srcs = [
        "environment_test.go",
        "types_test.go",
    ],
    embed = [":api"],
    deps = [
        "@io_k8s_api//core/v1:core",
        "@io_k8s_apimachinery//pkg/api/resource",
    ],
)
