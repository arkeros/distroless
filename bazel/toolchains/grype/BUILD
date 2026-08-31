load("@grype.bzl", "grype_toolchain")

grype_toolchain(
    name = "prebuilt_grype_toolchain",
    grype = "@multitool//tools/grype",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "grype",
    toolchain = ":prebuilt_grype_toolchain",
    toolchain_type = "@grype.bzl//grype:toolchain",
)
