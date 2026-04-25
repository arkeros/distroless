#!/usr/bin/env bash
# Generated wrapper for one (tf_root, verb) pair. Substitutions are filled
# in at analysis time by `_tf_runner` (see ./rule.bzl); after expansion
# the script is fully self-contained — no `bazel run`-only args injection,
# so `aspect plan` can spawn the wrapper directly via `runnable`.
set -euo pipefail

# Locate the runfiles dir. Both `bazel run` (sets RUNFILES_DIR) and
# `runnable.spawn` (also sets RUNFILES_DIR) cover the standard cases;
# the fallbacks handle a bare `./<script>` invocation too.
if [[ -z "${RUNFILES_DIR:-}" ]]; then
  for cand in "${BASH_SOURCE[0]}.runfiles" "$0.runfiles"; do
    if [[ -d "$cand" ]]; then
      RUNFILES_DIR="$cand"
      break
    fi
  done
fi
[[ -d "${RUNFILES_DIR:-}" ]] || { echo "tf-runner: cannot locate runfiles" >&2; exit 2; }
export RUNFILES_DIR

# The underlying run.sh expects paths relative to $PWD. Setting cwd to
# the workspace's runfiles subtree (`_main`) lets it resolve in-repo
# rootpaths (`infra/cloud/gcp/foo/...`) and external rootpaths
# (`../<repo>/...`) the same way `bazel run` would.
cd "$RUNFILES_DIR/_main"

exec ./devtools/build/tools/tf/run.sh \
  '{TERRAFORM_PATH}' \
  '{GEN_FILE}' \
  '{VERB}' \
  '{ROOT_NAME}' \
  -- \
  {TFVARS_ARGS} \
  -- \
  {MODULES_ARGS} \
  -- \
  {PRE_APPLY_ARGS}
