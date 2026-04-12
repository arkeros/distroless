load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "internal",
    srcs = [
        "load.go",
        "render.go",
    ],
    importpath = "github.com/arkeros/senku/devtools/bifrost/internal",
    visibility = ["//devtools/bifrost:__subpackages__"],
    deps = [
        "//devtools/bifrost/api",
        "@io_k8s_api//core/v1:core",
        "@io_k8s_apimachinery//pkg/util/intstr",
        "@io_k8s_sigs_yaml//:yaml",
    ],
)

go_test(
    name = "internal_test",
    srcs = ["render_test.go"],
    embed = [":internal"],
    deps = ["//devtools/bifrost/api"],
)
