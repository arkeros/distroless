load("@bazelrc-preset.bzl", "bazelrc_preset")
load("@rules_shell//shell:sh_library.bzl", "sh_library")

bazelrc_preset(
    name = "defaults",
    doc_link_template = "https://registry.build/flag/bazel?filter={flag}",
    strict = True,
)

# Run by Bazel as the --workspace_status_command, not as a target; an
# sh_library so shellcheck visits it under `--config=lint`.
sh_library(
    name = "workspace_status",
    srcs = ["workspace_status.sh"],
)
