#!/usr/bin/env bash
# Local-only: runs `terraform init -backend=false && terraform validate`.
# Not wrapped as a test because init needs network to download providers
# and the Bazel sandbox blocks it. Invoke via `bazel run`.
#
# Env: $TERRAFORM_BIN — absolute path to the terraform binary, set by the
#                       generating rule (see //devtools/build/tools/tf:lint.bzl).
#
# Args (optional): override the module path that's validated. Defaults to
# the module under `devtools/bifrost/terraform/modules/${TARGET_NAME:-service}`
# in the workspace.
set -euo pipefail

MODULE_DIR="$BUILD_WORKSPACE_DIRECTORY/devtools/bifrost/terraform/modules/${TARGET_NAME:-service}"
if [[ $# -gt 0 ]]; then
  MODULE_DIR="$1"
fi

cd "$MODULE_DIR"
"$TERRAFORM_BIN" init -backend=false -input=false
"$TERRAFORM_BIN" validate
