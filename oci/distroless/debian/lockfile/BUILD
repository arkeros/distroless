load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "lockfile",
    srcs = ["lockfile.go"],
    importpath = "github.com/arkeros/senku/oci/distroless/debian/lockfile",
    visibility = ["//visibility:public"],
)

go_test(
    name = "lockfile_test",
    srcs = ["lockfile_test.go"],
    data = glob(["testdata/**"]),
    embed = [":lockfile"],
)
