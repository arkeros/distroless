load("@grype.bzl", "grype_toolchain")

grype_toolchain(
    name = "compiled_grype_toolchain",
    grype = "@com_github_anchore_grype//cmd/grype",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "grype",
    toolchain = ":compiled_grype_toolchain",
    toolchain_type = "@grype.bzl//grype:toolchain",
)
