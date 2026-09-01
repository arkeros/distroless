load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "deb",
    srcs = ["deb.go"],
    importpath = "github.com/arkeros/distroless/oci/debian/deb",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_blakesmith_ar//:ar",
        "@com_github_klauspost_compress//zstd",
        "@com_github_ulikunitz_xz//:xz",
    ],
)

go_test(
    name = "deb_test",
    srcs = ["deb_test.go"],
    deps = [
        ":deb",
        "@com_github_blakesmith_ar//:ar",
    ],
)
