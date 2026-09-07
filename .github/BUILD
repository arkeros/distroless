load("@rules_shell//shell:sh_library.bzl", "sh_library")

# The Renovate container's entrypoint (workflows/renovate.yaml), which Bazel
# never runs; an sh_library so shellcheck visits it under `--config=lint`.
sh_library(
    name = "renovate_entrypoint",
    srcs = ["renovate-entrypoint.sh"],
)
