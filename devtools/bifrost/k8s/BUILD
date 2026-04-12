load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "k8s",
    srcs = ["k8s.go"],
    importpath = "github.com/arkeros/senku/devtools/bifrost/k8s",
    visibility = ["//visibility:public"],
    deps = [
        "//devtools/bifrost/api",
        "//devtools/bifrost/internal",
        "//platform/kubernetes/secrets/providers/gcp",
        "@io_k8s_api//apps/v1:apps",
        "@io_k8s_api//autoscaling/v2:autoscaling",
        "@io_k8s_api//batch/v1:batch",
        "@io_k8s_api//core/v1:core",
        "@io_k8s_api//policy/v1:policy",
        "@io_k8s_apimachinery//pkg/apis/meta/v1:meta",
        "@io_k8s_apimachinery//pkg/util/intstr",
    ],
)

go_test(
    name = "k8s_test",
    srcs = ["k8s_test.go"],
    data = glob(["testdata/**"]),
    embed = [":k8s"],
    deps = [
        "//devtools/bifrost/testing",
        "//testing/golden",
    ],
)
