load("@kustomize.bzl", "kustomize_toolchain")

kustomize_toolchain(
    name = "compiled_kustomize_toolchain",
    kustomize = "@io_k8s_sigs_kustomize_kustomize_v5//:v5",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "kustomize",
    toolchain = ":compiled_kustomize_toolchain",
    toolchain_type = "@kustomize.bzl//:toolchain_type",
)
