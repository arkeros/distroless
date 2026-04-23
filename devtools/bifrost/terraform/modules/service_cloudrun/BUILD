load("@rules_shell//shell:sh_binary.bzl", "sh_binary")
load("@rules_shell//shell:sh_test.bzl", "sh_test")

_HCL_SRCS = glob(
    ["**/*.tf"],
    allow_empty = False,
)

filegroup(
    name = "module",
    srcs = _HCL_SRCS,
    visibility = ["//visibility:public"],
)

# Lint, not a test. `terraform fmt -check -recursive` in the Bazel sandbox.
sh_test(
    name = "lint",
    size = "small",
    srcs = ["//devtools/bifrost/terraform/modules:fmt_check.sh"],
    args = ["$(rootpath @multitool//tools/terraform)"],
    data = _HCL_SRCS + ["@multitool//tools/terraform"],
)

sh_binary(
    name = "validate",
    srcs = ["//devtools/bifrost/terraform/modules:validate.sh"],
    args = ["$(rootpath @multitool//tools/terraform)"],
    data = _HCL_SRCS + ["@multitool//tools/terraform"],
)
