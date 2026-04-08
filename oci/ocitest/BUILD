load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "ocitest",
    testonly = True,
    srcs = ["fakeregistry.go"],
    importpath = "github.com/arkeros/senku/oci/ocitest",
    visibility = ["//oci:__subpackages__"],
)
