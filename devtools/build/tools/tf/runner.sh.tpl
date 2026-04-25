#!/usr/bin/env bash
# Generated wrapper for one (tf_root, verb) pair. Substitutions are filled
# in at analysis time by `_tf_runner` (see ./rule.bzl); after expansion
# the script is fully self-contained — no `bazel run`-only args injection,
# so `aspect plan` can spawn the wrapper directly via `runnable`.
#
# Path resolution goes through Bazel's bash runfiles library, so this
# script works whether the runfiles symlink tree was materialized
# (`--build_runfile_links`) or only the manifest is on disk
# (`--nobuild_runfile_links`, the workspace default).
set -euo pipefail

# --- begin runfiles.bash initialization v3 ---
# Copy-pasted from the Bazel Bash runfiles library v3.
set -uo pipefail; set +e; f=bazel_tools/tools/bash/runfiles/runfiles.bash
source "${RUNFILES_DIR:-/dev/null}/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "${RUNFILES_MANIFEST_FILE:-/dev/null}" | cut -f 2- -d ' ')" 2>/dev/null || \
  source "$0.runfiles/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.runfiles_manifest" | cut -f 2- -d ' ')" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.exe.runfiles_manifest" | cut -f 2- -d ' ')" 2>/dev/null || \
  { echo>&2 "ERROR: cannot find $f"; exit 1; }; f=; set -e
# --- end runfiles.bash initialization v3 ---

# Newline-separated rlocation paths consumed by run.sh. Forwarded through
# the env so run.sh's positional argv stays compact and stable.
export TFRUNNER_GEN_FILES='{GEN_FILES_NL}'
export TFRUNNER_TFVARS='{TFVARS_NL}'
export TFRUNNER_MODULES='{MODULES_NL}'
export TFRUNNER_PRE_APPLY='{PRE_APPLY_NL}'

# Export RUNFILES_DIR / RUNFILES_MANIFEST_FILE so run.sh's own runfiles
# init (and any pre-apply hook's) finds the same source of truth instead
# of re-discovering it via $0.runfiles fallbacks.
runfiles_export_envvars

exec "$(rlocation '{RUN_SH_PATH}')" \
  "$(rlocation '{TERRAFORM_PATH}')" \
  '{VERB}' \
  '{ROOT_NAME}' \
  "$@"
