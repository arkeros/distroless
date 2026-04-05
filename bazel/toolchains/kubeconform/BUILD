load("@kubeconform.bzl", "kubeconform_toolchain")

kubeconform_toolchain(
    name = "compiled_kubeconform_toolchain",
    kubeconform = "@com_github_yannh_kubeconform//cmd/kubeconform",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "kubeconform",
    toolchain = ":compiled_kubeconform_toolchain",
    toolchain_type = "@kubeconform.bzl//:toolchain_type",
)
