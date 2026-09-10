load("@rules_shell//shell:sh_library.bzl", "sh_library")

# The Renovate container's entrypoint (workflows/renovate.yaml), which Bazel
# never runs; an sh_library so shellcheck visits it under `--config=lint`.
sh_library(
    name = "renovate_entrypoint",
    srcs = ["renovate-entrypoint.sh"],
)

# The pull-request roll-up of target-determinator's output
# (workflows/pr.yaml). Bazel never runs it either — it runs Bazel — but an
# sh_library puts it in front of shellcheck under `--config=lint`.
sh_library(
    name = "affected_targets",
    srcs = ["affected-targets.sh"],
)
