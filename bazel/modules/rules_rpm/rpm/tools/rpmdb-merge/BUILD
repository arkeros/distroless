load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "rpmdb-merge_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/bazel/modules/rules_rpm/rpm/tools/rpmdb-merge",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_klauspost_compress//zstd",
        "@org_modernc_sqlite//:sqlite",
    ],
)

go_binary(
    name = "rpmdb-merge",
    embed = [":rpmdb-merge_lib"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "rpmdb-merge_test",
    srcs = ["main_test.go"],
    data = ["//rpm/tools/rpmdb-merge/testdata:tzdata.header.blob"],
    embed = [":rpmdb-merge_lib"],
    deps = ["@org_modernc_sqlite//:sqlite"],
)
