#!/usr/bin/env bash
# Per-root terraform runner. Generated `.tf.json` files live in the read-only
# Bazel runfiles tree; copy them to a stable workdir keyed by the root name so
# terraform's mutable state (.terraform/, lockfiles) survives between runs and
# two roots don't fight over the same directory.
#
# Args:
#   $1  — terraform binary (already resolved to an absolute path by the wrapper)
#   $2  — verb: plan | apply | destroy
#   $3  — root name (used as the workdir key)
#   $4+ — extra flags forwarded to the terraform invocation
#
# Env (newline-separated rlocation paths; unset/empty means "none"):
#   TFRUNNER_GEN_FILES — generated `*.tf.json` files
#   TFRUNNER_TFVARS    — `*.auto.tfvars.json` files
#   TFRUNNER_MODULES   — `<subdir>|<relpath>|<rloc>` triples (one per file)
#   TFRUNNER_PRE_APPLY — pre-apply executables (only run on `apply`)
#
# Env (control):
#   TF_WORKDIR       — base directory for per-root workspaces (default ~/.cache/senku-tf)
#   TF_AUTO_APPROVE  — if non-empty, pass `-auto-approve` to apply/destroy
#   TF_INIT_UPGRADE  — if non-empty, pass `-upgrade` to init (refresh lockfile)
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

TERRAFORM_BIN="$1"
VERB="$2"
ROOT_NAME="$3"
shift 3

# Bazel labels can't contain newlines or whitespace, so newline-separated
# lists are unambiguous. The `-n` guard avoids a stray empty element when
# the var is unset or empty.
GEN_FILES=()
if [[ -n "${TFRUNNER_GEN_FILES:-}" ]]; then
  while IFS= read -r line; do GEN_FILES+=("$line"); done <<< "$TFRUNNER_GEN_FILES"
fi

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

# Strip ANSI colors when stdout isn't a terminal — i.e. CI capture to a file
# for the PR-comment upload. Terraform doesn't auto-detect this reliably.
NO_COLOR=()
if ! [ -t 1 ]; then
  NO_COLOR=(-no-color)
fi

WORK="${TF_WORKDIR:-$HOME/.cache/senku-tf}/$ROOT_NAME"
mkdir -p "$WORK"

# Wipe Bazel-owned files (everything we generate) before re-syncing. State,
# lockfiles, and `.terraform/` survive.
find "$WORK" -maxdepth 1 -name "*.tf.json" -delete
find "$WORK" -maxdepth 1 -name "*.auto.tfvars.json" -delete
# Module subdirs are also Bazel-owned; nuke each one once before re-staging.
declare -A SEEN_SUBDIRS=()
for entry in "${MODULES[@]}"; do
  subdir="${entry%%|*}"
  if [[ -z "${SEEN_SUBDIRS[$subdir]:-}" ]]; then
    rm -rf "${WORK:?}/$subdir"
    SEEN_SUBDIRS["$subdir"]=1
  fi
done

for f in "${GEN_FILES[@]}"; do
  cp "$(rlocation "$f")" "$WORK/"
done

for tf in "${TFVARS[@]}"; do
  cp "$(rlocation "$tf")" "$WORK/"
done

for entry in "${MODULES[@]}"; do
  IFS='|' read -r subdir relpath rloc <<< "$entry"
  src="$(rlocation "$rloc")"
  dest="$WORK/$subdir/$relpath"
  mkdir -p "$(dirname "$dest")"
  cp "$src" "$dest"
done

if [[ "$VERB" == "apply" ]]; then
  # Pre-apply hooks have their own runfiles trees as part of THIS binary's
  # runfiles. Export RUNFILES_DIR / _MANIFEST_FILE so each hook's own
  # runfiles library finds them via env instead of an absent `<argv[0]>.runfiles`.
  runfiles_export_envvars
  for hook in "${PRE_APPLY[@]}"; do
    hook_path="$(rlocation "$hook")"
    echo "==> pre-apply: $hook_path"
    "$hook_path"
  done
fi

cd "$WORK"
# `init` (no `-upgrade`) uses the existing lockfile if present, picks the
# latest matching provider on first run. Set `TF_INIT_UPGRADE=1` to force
# `-upgrade` (refresh the lockfile to the latest matching versions).
"$TERRAFORM_BIN" init -input=false "${NO_COLOR[@]}" ${TF_INIT_UPGRADE:+-upgrade}

case "$VERB" in
  plan)
    # `-input=false` keeps plan non-blocking: a missing variable fails fast
    # instead of stalling on a terminal prompt.
    exec "$TERRAFORM_BIN" plan -input=false "${NO_COLOR[@]}" "$@"
    ;;
  apply|destroy)
    # No `-input=false` here: the y/n confirmation prompt is the default
    # human path. Set `TF_AUTO_APPROVE=1` to skip it (CI, scripted use).
    exec "$TERRAFORM_BIN" "$VERB" "${NO_COLOR[@]}" ${TF_AUTO_APPROVE:+-auto-approve} "$@"
    ;;
  *)
    echo "tf/run.sh: unknown verb '$VERB' (expected plan|apply|destroy)" >&2
    exit 2
    ;;
esac
