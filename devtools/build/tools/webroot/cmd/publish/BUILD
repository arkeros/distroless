load("@rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "publish_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/devtools/build/tools/webroot/cmd/publish",
    visibility = ["//visibility:private"],
    deps = [
        "//devtools/build/tools/webroot",
        "@com_google_cloud_go_storage//:storage",
        "@org_golang_google_api//iterator",
    ],
)

go_binary(
    name = "publish",
    embed = [":publish_lib"],
    visibility = ["//visibility:public"],
)
