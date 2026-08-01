load("@aspect_rules_py//py:defs.bzl", "py_binary", "py_library")
load("@gazelle//:go_tools.bzl", "GO_TOOLS")
load("@lazy_bazel_env.bzl//:defs.bzl", "lazy_bazel_env")
load("@multitool//:tools.bzl", MULTITOOLS = "TOOLS")

# Tools found in this package
LOCAL_TOOLS = [
    "b3sum",
]

# Tools found at @[toolname]
REPO_TOOLS = [
    "pnpm",
]

lazy_bazel_env(
    name = "dev",
    toolchains = {
        "python": "@rules_python//python:current_py_toolchain",
        "nodejs": "@nodejs_toolchains//:resolved_toolchain",
        "jq": "@jq_toolchains//:resolved_toolchain",
    },
    tools = {
        # Eager tools - resolved from toolchain Make variables
        # "jar": "$(JAVABASE)/bin/jar",
        # "java": "$(JAVA)",
        "node": "$(NODE_PATH)",
        "python": "$(PYTHON3)",
        "jq": "$(JQ_BIN)",
        # Lazy tools - compiled on first use
        "buildifier": "@com_github_bazelbuild_buildtools//buildifier",
        "buildozer": "@com_github_bazelbuild_buildtools//buildozer",
        "crane": "@com_github_google_go_containerregistry_cmd_krane//:krane",
        # https://github.com/bazelbuild/rules_go/blob/master/docs/go/core/bzlmod.md#using-a-go-sdk
        "go": "@rules_go//go",
        "gazelle": "//:gazelle",
        # "flake8": "@pip//flake8:pkg",
        # "prettier": "//tools/format:prettier",
        "bifrost": "//devtools/bifrost/cli",
        "knife": "//bazel/cmd/knife",
    } | {
        t: "@multitool//tools/" + t
        for t in MULTITOOLS
    } | {
        t: "@" + t
        for t in REPO_TOOLS
    } | {
        t: "//tools:" + t
        for t in LOCAL_TOOLS
    } | GO_TOOLS,
)

py_binary(
    name = "b3sum",
    srcs = ["b3sum.py"],
    visibility = ["//:__subpackages__"],
    deps = ["@pip//blake3"],
)

py_library(
    name = "tools",
    srcs = ["b3sum.py"],
    visibility = ["//:__subpackages__"],
    deps = ["@pip//blake3"],
)
