#!/usr/bin/env bash
# Per-root terraform runner. Generated `.tf.json` files live in the read-only
# Bazel runfiles tree; copy them to a stable workdir keyed by the root name so
# terraform's mutable state (.terraform/, lockfiles) survives between runs and
# two roots don't fight over the same directory.
#
# Args (`--` separates sections; sections may be empty):
#   $1 — runfiles path to the terraform binary (from @multitool//tools/terraform)
#   $2 — runfiles path to ANY of the generated files (its dirname holds them all)
#   $3 — verb: plan | apply | destroy
#   $4 — root name (used as the workdir key)
#   $5 — literal `--`
#   ... — rootpaths to *.auto.tfvars.json files (zero or more)
#   --
#   ... — `subdir|package_path` pairs for modules (zero or more)
#   --
#   ... — rootpaths to pre-apply executables (zero or more; only run on `apply`)
#
# Env:
#   TF_WORKDIR       — base directory for per-root workspaces (default ~/.cache/senku-tf)
#   TF_AUTO_APPROVE  — if non-empty, pass `-auto-approve` to apply/destroy
#   TF_INIT_UPGRADE  — if non-empty, pass `-upgrade` to init (refresh lockfile)
set -euo pipefail

TERRAFORM_BIN="$PWD/$1"
GEN_FILE="$2"
VERB="$3"
ROOT_NAME="$4"
shift 4

# Consume the leading `--`.
[[ "${1:-}" == "--" ]] || { echo "tf/run.sh: expected '--' before tfvars"; exit 2; }
shift

TFVARS=()
while [[ "${1:-}" != "--" ]]; do
  TFVARS+=("$1")
  shift || break
done
shift  # consume the `--`

MODULES=()
while [[ "${1:-}" != "--" ]]; do
  MODULES+=("$1")
  shift || break
done
shift

PRE_APPLY=("$@")
# Clear positional args so the trailing pre-apply rootpaths don't leak into
# the terraform invocation below (which forwards `"$@"` for future user args).
set --

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
