load("@helm.bzl", "helm_toolchain")

helm_toolchain(
    name = "compiled_helm_toolchain",
    helm = "@sh_helm_helm_v4//cmd/helm",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "helm",
    toolchain = ":compiled_helm_toolchain",
    toolchain_type = "@helm.bzl//:toolchain_type",
)
