load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "cloudrun",
    srcs = ["cloudrun.go"],
    importpath = "github.com/arkeros/senku/devtools/bifrost/cloudrun",
    visibility = ["//visibility:public"],
    deps = [
        "//devtools/bifrost/api",
        "//devtools/bifrost/internal",
        "//platform/secrets/gcp",
        "@dev_knative_serving//pkg/apis/serving/v1:serving",
        "@io_k8s_api//core/v1:core",
        "@io_k8s_apimachinery//pkg/apis/meta/v1:meta",
    ],
)

go_test(
    name = "cloudrun_test",
    srcs = ["cloudrun_test.go"],
    data = glob(["testdata/**"]),
    embed = [":cloudrun"],
    deps = [
        "//devtools/bifrost/api",
        "//devtools/bifrost/testing",
        "//testing/golden",
        "@io_k8s_api//core/v1:core",
        "@io_k8s_apimachinery//pkg/api/resource",
    ],
)
