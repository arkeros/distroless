load("@syft.bzl//syft/toolchain:toolchain.bzl", "syft_toolchain")

syft_toolchain(
    name = "compiled_syft_toolchain",
    syft = "@com_github_anchore_syft//cmd/syft",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "syft",
    toolchain = ":compiled_syft_toolchain",
    toolchain_type = "@syft.bzl//syft:toolchain",
)
