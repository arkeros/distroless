# Package marker so the `include("//bazel/include:*.MODULE.bazel")` calls in
# //:MODULE.bazel resolve as labels.

# The included files are in no bzl_library; the `starlark` tag has
# buildifier's lint (//tools/lint:linters.bzl) visit them here.
filegroup(
    name = "module_files",
    srcs = glob(["*.MODULE.bazel"]),
    tags = ["starlark"],
)
