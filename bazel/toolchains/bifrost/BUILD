load("//devtools/bifrost/toolchain:toolchain.bzl", "bifrost_toolchain")

bifrost_toolchain(
    name = "compiled_bifrost_toolchain",
    bifrost = "//devtools/bifrost/cli",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "bifrost",
    toolchain = ":compiled_bifrost_toolchain",
    toolchain_type = "//devtools/bifrost/toolchain:toolchain_type",
)
