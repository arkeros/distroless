load("@rules_go//go:def.bzl", "go_library", "go_test")

# The per-install pin binary is materialized inside each
# `@<install_name>` hub repo (see `//terraform:extensions.bzl`).
# Each hub repo declares its own `go_binary(name = "pin", embed =
# [":pin_lib"], args = [...])` with the lockfile path + provider list
# baked into args — so the consumer-facing invocation is just
# `bazel run @<install>//:pin`, no extra flags.
go_library(
    name = "pin_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/bazel/modules/terraform.bzl/terraform/private/pin",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_hashicorp_go_version//:go-version",
        "@org_golang_x_mod//sumdb/dirhash",
    ],
)

go_test(
    name = "pin_test",
    srcs = ["main_test.go"],
    embed = [":pin_lib"],
    deps = ["@com_github_hashicorp_go_version//:go-version"],
)
