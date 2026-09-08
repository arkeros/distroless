load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "tarballs",
    srcs = [
        "github.go",
        "http.go",
        "lockfile.go",
        "nodejs.go",
        "temurin.go",
        "update.go",
    ],
    importpath = "github.com/arkeros/distroless/bazel/tarballs",
    visibility = ["//visibility:public"],
)

go_test(
    name = "tarballs_test",
    srcs = ["tarballs_test.go"],
    embed = [":tarballs"],
)

bzl_library(
    name = "extensions",
    srcs = ["extensions.bzl"],
    visibility = ["//visibility:public"],
    deps = ["@bazel_tools//tools/build_defs/repo:http.bzl"],
)
