load("@cosign.bzl", "cosign_toolchain")

cosign_toolchain(
    name = "compiled_cosign_toolchain",
    cosign = "@com_github_sigstore_cosign_v3//cmd/cosign",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "cosign",
    toolchain = ":compiled_cosign_toolchain",
    toolchain_type = "@cosign.bzl//cosign:toolchain",
)
