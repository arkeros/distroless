load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "gcp",
    srcs = ["gcp.go"],
    importpath = "github.com/arkeros/senku/platform/secrets/gcp",
    visibility = ["//visibility:public"],
    deps = [
        "@com_google_cloud_go_secretmanager//apiv1",
        "@com_google_cloud_go_secretmanager//apiv1/secretmanagerpb",
    ],
)

go_test(
    name = "gcp_test",
    srcs = ["gcp_test.go"],
    embed = [":gcp"],
)
