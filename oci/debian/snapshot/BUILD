load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "snapshot",
    srcs = [
        "fetch.go",
        "snapshot.go",
    ],
    importpath = "github.com/arkeros/distroless/oci/debian/snapshot",
    visibility = ["//visibility:public"],
    deps = [
        "@in_yaml_go_yaml_v4//:yaml",
    ],
)

go_test(
    name = "snapshot_test",
    srcs = [
        "fetch_test.go",
        "snapshot_test.go",
    ],
    data = glob(["testdata/**"]),
    embed = [":snapshot"],
)
