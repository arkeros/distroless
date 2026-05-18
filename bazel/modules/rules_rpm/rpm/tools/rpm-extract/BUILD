load("@rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "rpm-extract_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/bazel/modules/rules_rpm/rpm/tools/rpm-extract",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_protonmail_go_crypto//openpgp",
        "@com_github_sassoftware_go_rpmutils//:go-rpmutils",
        "@com_github_sassoftware_go_rpmutils//cpio",
    ],
)

go_binary(
    name = "rpm-extract",
    embed = [":rpm-extract_lib"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "rpm-extract_test",
    srcs = ["main_test.go"],
    data = [
        "//rpm/tools/rpm-extract/testdata:hummingbird-release.pgp",
        "//rpm/tools/rpm-extract/testdata:tzdata.rpm",
    ],
    embed = [":rpm-extract_lib"],
)
