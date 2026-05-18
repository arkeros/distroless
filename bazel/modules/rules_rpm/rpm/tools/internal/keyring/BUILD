load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "keyring",
    srcs = ["keyring.go"],
    importpath = "github.com/arkeros/senku/bazel/modules/rules_rpm/rpm/tools/internal/keyring",
    visibility = ["//rpm/tools:__subpackages__"],
    deps = ["@com_github_protonmail_go_crypto//openpgp"],
)

go_test(
    name = "keyring_test",
    srcs = ["keyring_test.go"],
    embed = [":keyring"],
    deps = [
        "@com_github_protonmail_go_crypto//openpgp",
        "@com_github_protonmail_go_crypto//openpgp/armor",
    ],
)
