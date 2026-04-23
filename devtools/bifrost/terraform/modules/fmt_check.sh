#!/usr/bin/env bash
# Hermetic HCL format check. Runs `terraform fmt -check -recursive` against
# every .tf file packaged as test data.
#
# Args: $1 = rootpath to the terraform binary (from @multitool//tools/terraform).
#
# Why only fmt: `terraform init` needs network to download providers and the
# Bazel sandbox has none. `terraform validate` requires init. fmt is offline
# and still catches HCL syntax errors.
set -euo pipefail

TERRAFORM="$PWD/$1"
shift

# Discover the module root — the directory containing this test's data tree.
MODULE_ROOT=""
for tf in "$TEST_SRCDIR"/_main/devtools/bifrost/terraform/modules/*/; do
  # Match the module whose files are under this test's runfiles.
  if compgen -G "${tf}*.tf" > /dev/null; then
    MODULE_ROOT="$tf"
    break
  fi
done

if [[ -z "$MODULE_ROOT" ]]; then
  # Fall back: find any directory with a .tf file and recurse from there.
  MODULE_ROOT="$(find "$TEST_SRCDIR/_main/devtools/bifrost/terraform/modules" -name "*.tf" -printf "%h\n" | sort -u | head -n1)"
fi

echo "terraform fmt -check -recursive $MODULE_ROOT"
"$TERRAFORM" fmt -check -recursive "$MODULE_ROOT"
