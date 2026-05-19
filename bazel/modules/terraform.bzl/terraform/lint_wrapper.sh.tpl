#!/usr/bin/env bash
# Generated wrapper. Resolves the registered terraform toolchain at
# Bazel analysis time and exports `TERRAFORM_BIN` for the underlying
# script (e.g. fmt_check.sh / validate.sh). Self-contained — no `bazel
# run`-only args injection.
set -euo pipefail

if [[ -z "${RUNFILES_DIR:-}" ]]; then
  for cand in "${BASH_SOURCE[0]}.runfiles" "$0.runfiles" "${TEST_SRCDIR:-}"; do
    if [[ -d "$cand" ]]; then
      RUNFILES_DIR="$cand"
      break
    fi
  done
fi
[[ -d "${RUNFILES_DIR:-}" ]] || { echo "tf-lint: cannot locate runfiles" >&2; exit 2; }
export RUNFILES_DIR

cd "$RUNFILES_DIR/_main"

# Terraform's `short_path` may start with `../` for an external repo;
# `$PWD` (set above to the `_main` subtree) makes that resolve correctly.
export TERRAFORM_BIN="$PWD/{TERRAFORM_PATH}"

exec "$PWD/{SCRIPT_PATH}" "$@"
