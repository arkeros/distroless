#!/usr/bin/env bash
# Local-only: runs `terraform init -backend=false && terraform validate`.
# Not wrapped in sh_test because init needs network to download providers
# and the Bazel sandbox blocks it. Invoke via `bazel run`.
set -euo pipefail

TERRAFORM="$BUILD_WORKSPACE_DIRECTORY/$(realpath --relative-to="$BUILD_WORKSPACE_DIRECTORY" "$1")"
shift

MODULE_DIR="$BUILD_WORKSPACE_DIRECTORY/devtools/bifrost/terraform/modules/${TARGET_NAME:-service}"

# Allow override: bazel run ... -- /path/to/module
if [[ $# -gt 0 ]]; then
  MODULE_DIR="$1"
fi

cd "$MODULE_DIR"
"$TERRAFORM" init -backend=false -input=false
"$TERRAFORM" validate
