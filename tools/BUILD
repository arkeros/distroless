load("@gazelle//:go_tools.bzl", "GO_TOOLS")
load("@lazy_bazel_env.bzl//:defs.bzl", "lazy_bazel_env")
load("@multitool//:tools.bzl", MULTITOOLS = "TOOLS")

lazy_bazel_env(
    name = "dev",
    toolchains = {
        "jq": "@jq_toolchains//:resolved_toolchain",
    },
    tools = {
        # Eager tools - resolved from toolchain Make variables
        # "jar": "$(JAVABASE)/bin/jar",
        # "java": "$(JAVA)",
        "jq": "$(JQ_BIN)",
        # Lazy tools - compiled on first use
        "buildifier": "@com_github_bazelbuild_buildtools//buildifier",
        "buildozer": "@com_github_bazelbuild_buildtools//buildozer",
        "crane": "@com_github_google_go_containerregistry_cmd_krane//:krane",
        # https://github.com/bazelbuild/rules_go/blob/master/docs/go/core/bzlmod.md#using-a-go-sdk
        "go": "@rules_go//go",
        "gazelle": "//:gazelle",
        # "prettier": "//tools/format:prettier",
        "knife": "//bazel/cmd/knife",
    } | {
        t: "@multitool//tools/" + t
        for t in MULTITOOLS
    } | GO_TOOLS,
)
