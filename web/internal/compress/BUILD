load("@rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//web:__subpackages__"])

go_library(
    name = "compress",
    srcs = ["compress.go"],
    importpath = "github.com/arkeros/distroless/web/internal/compress",
    deps = ["@com_github_cafxx_httpcompression//:httpcompression"],
)

go_test(
    name = "compress_test",
    size = "small",
    srcs = ["compress_test.go"],
    deps = [":compress"],
)
