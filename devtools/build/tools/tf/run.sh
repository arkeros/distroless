#!/usr/bin/env bash
# Per-root terraform runner. Generated `.tf.json` files live in the read-only
# Bazel runfiles tree; copy them to a stable workdir keyed by the root name so
# terraform's mutable state (.terraform/, lockfiles) survives between runs and
# two roots don't fight over the same directory.
#
# Args:
#   $1  — runfiles path to the terraform binary (from @multitool//tools/terraform)
#   $2  — runfiles path to ANY of the generated files (its dirname holds them all)
#   $3  — verb: plan | apply | destroy
#   $4  — root name (used as the workdir key)
#
# Env (newline-separated lists; unset/empty means "none"):
#   TFRUNNER_TFVARS    — rootpaths to *.auto.tfvars.json files
#   TFRUNNER_MODULES   — `subdir|package_path` pairs for modules
#   TFRUNNER_PRE_APPLY — rootpaths to pre-apply executables (only run on `apply`)
#
# Env (control):
#   TF_WORKDIR       — base directory for per-root workspaces (default ~/.cache/senku-tf)
#   TF_AUTO_APPROVE  — if non-empty, pass `-auto-approve` to apply/destroy
#   TF_INIT_UPGRADE  — if non-empty, pass `-upgrade` to init (refresh lockfile)
set -euo pipefail

TERRAFORM_BIN="$PWD/$1"
GEN_FILE="$2"
VERB="$3"
ROOT_NAME="$4"
shift 4

# Bazel labels can't contain newlines or whitespace, so newline-separated
# lists are unambiguous. The `-n` guard avoids a stray empty element when
# the var is unset or empty.
TFVARS=()
if [[ -n "${TFRUNNER_TFVARS:-}" ]]; then
  while IFS= read -r line; do TFVARS+=("$line"); done <<< "$TFRUNNER_TFVARS"
fi

MODULES=()
if [[ -n "${TFRUNNER_MODULES:-}" ]]; then
  while IFS= read -r line; do MODULES+=("$line"); done <<< "$TFRUNNER_MODULES"
fi

PRE_APPLY=()
if [[ -n "${TFRUNNER_PRE_APPLY:-}" ]]; then
  while IFS= read -r line; do PRE_APPLY+=("$line"); done <<< "$TFRUNNER_PRE_APPLY"
fi

GEN_DIR="$PWD/$(dirname "$GEN_FILE")"
WORK="${TF_WORKDIR:-$HOME/.cache/senku-tf}/$ROOT_NAME"

mkdir -p "$WORK"

# Wipe Bazel-owned files (everything we generate) before re-syncing. State,
# lockfiles, and `.terraform/` survive.
find "$WORK" -maxdepth 1 -name "*.tf.json" -delete
find "$WORK" -maxdepth 1 -name "*.auto.tfvars.json" -delete
# Module subdirs are also Bazel-owned; nuke whatever was there last run.
for entry in "${MODULES[@]}"; do
  subdir="${entry%%|*}"
  rm -rf "${WORK:?}/$subdir"
done

cp "$GEN_DIR"/*.tf.json "$WORK/"

for tf in "${TFVARS[@]}"; do
  cp "$PWD/$tf" "$WORK/"
done

for entry in "${MODULES[@]}"; do
  subdir="${entry%%|*}"
  pkg="${entry#*|}"
  mkdir -p "$WORK/$subdir"
  # `data = filegroup` ensures only the module's declared files are in
  # runfiles at this package path, so cp -r picks up exactly the right set.
  cp -R "$PWD/$pkg/." "$WORK/$subdir/"
done

if [[ "$VERB" == "apply" ]]; then
  # `pre_apply` children are part of THIS binary's runfiles tree (they're
  # declared as `data`). Their own runfiles libraries look for
  # `RUNFILES_DIR` / `RUNFILES_MANIFEST_FILE` env vars; without them they
  # fall back to `<argv[0]>.runfiles`, which doesn't exist because the
  # child is INSIDE our runfiles tree, not next to its own. Point them at
  # our tree (the parent dir of `_main`).
  export RUNFILES_DIR="${RUNFILES_DIR:-$(cd .. && pwd)}"
  for hook in "${PRE_APPLY[@]}"; do
    echo "==> pre-apply: $hook"
    "$PWD/$hook"
  done
fi

cd "$WORK"
# `init` (no `-upgrade`) uses the existing lockfile if present, picks the
# latest matching provider on first run. Set `TF_INIT_UPGRADE=1` to force
# `-upgrade` (refresh the lockfile to the latest matching versions).
"$TERRAFORM_BIN" init -input=false ${TF_INIT_UPGRADE:+-upgrade}

case "$VERB" in
  plan)
    # `-input=false` keeps plan non-blocking: a missing variable fails fast
    # instead of stalling on a terminal prompt.
    exec "$TERRAFORM_BIN" plan -input=false "$@"
    ;;
  apply|destroy)
    # No `-input=false` here: the y/n confirmation prompt is the default
    # human path. Set `TF_AUTO_APPROVE=1` to skip it (CI, scripted use).
    exec "$TERRAFORM_BIN" "$VERB" ${TF_AUTO_APPROVE:+-auto-approve} "$@"
    ;;
  *)
    echo "tf/run.sh: unknown verb '$VERB' (expected plan|apply|destroy)" >&2
    exit 2
    ;;
esac
