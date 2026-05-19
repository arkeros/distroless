#!/usr/bin/env bash
# `bazel run @<install>//:pin` — regenerate `.terraform.lock.hcl` for every
# platform we support. Wraps the terraform binary from the registered
# toolchain so consumers don't need terraform installed locally.
#
# Substitutions filled in by `_hub_repo_impl` (see `extensions.bzl`):
#   {LOCK_DIR_REL}  — workspace-relative path of the dir holding
#                     `versions.tf` + `.terraform.lock.hcl`.
set -euo pipefail

# --- begin runfiles.bash initialization v3 ---
set -uo pipefail; set +e; f=bazel_tools/tools/bash/runfiles/runfiles.bash
source "${RUNFILES_DIR:-/dev/null}/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "${RUNFILES_MANIFEST_FILE:-/dev/null}" | cut -f 2- -d ' ')" 2>/dev/null || \
  source "$0.runfiles/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.runfiles_manifest" | cut -f 2- -d ' ')" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.exe.runfiles_manifest" | cut -f 2- -d ' ')" 2>/dev/null || \
  { echo>&2 "ERROR: cannot find $f"; exit 1; }; f=; set -e
# --- end runfiles.bash initialization v3 ---

if [[ -z "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
  echo "pin: must be run via 'bazel run @<install_name>//:pin' (BUILD_WORKSPACE_DIRECTORY unset)" >&2
  exit 1
fi

# rlocation resolves the apparent name `terraform_toolchains` via the hub
# repo's repo_mapping (both hub and toolchains repos come from the same
# module extension, so the mapping is in scope).
TERRAFORM_BIN="$(rlocation 'terraform_toolchains/terraform_bin')"
if [[ -z "$TERRAFORM_BIN" || ! -x "$TERRAFORM_BIN" ]]; then
  echo "pin: cannot resolve terraform_toolchains/terraform_bin via rlocation" >&2
  exit 1
fi

LOCK_DIR="$BUILD_WORKSPACE_DIRECTORY/{LOCK_DIR_REL}"
echo "pin: re-locking providers in $LOCK_DIR" >&2

# `-chdir` is the global terraform flag, applied before the subcommand.
# `terraform providers lock -platform=X` writes hashes for platform X into
# `.terraform.lock.hcl`; repeating it accumulates all platforms in one
# file. The four below cover what `_PROVIDER_PLATFORMS` in the extension
# materializes.
exec "$TERRAFORM_BIN" -chdir="$LOCK_DIR" providers lock \
  -platform=darwin_amd64 \
  -platform=darwin_arm64 \
  -platform=linux_amd64 \
  -platform=linux_arm64
