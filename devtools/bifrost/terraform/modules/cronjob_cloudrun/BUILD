load("//devtools/build/tools/tf:lint.bzl", "tf_script_binary", "tf_script_test")

_HCL_SRCS = glob(
    ["**/*.tf"],
    allow_empty = False,
)

filegroup(
    name = "module",
    srcs = _HCL_SRCS,
    visibility = ["//visibility:public"],
)

# Lint, not a test. Semantic validation requires provider downloads; see
# :validate for the local-only target.
tf_script_test(
    name = "lint",
    size = "small",
    srcs = _HCL_SRCS,
    script = "//devtools/bifrost/terraform/modules:fmt_check.sh",
)

tf_script_binary(
    name = "validate",
    srcs = _HCL_SRCS,
    script = "//devtools/bifrost/terraform/modules:validate.sh",
)
