load("@cosign.bzl", "cosign_toolchain")

cosign_toolchain(
    name = "prebuilt_cosign_toolchain",
    cosign = "@multitool//tools/cosign",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "cosign",
    toolchain = ":prebuilt_cosign_toolchain",
    toolchain_type = "@cosign.bzl//cosign:toolchain",
)
