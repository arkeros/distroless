#!/usr/bin/env bash
# Per-root terraform runner. The bazel-bin output directory of the
# generating `tf_root` IS the terraform working directory — it already
# contains main.tf.json, backend.tf.json, providers.tf.json,
# .terraform.lock.hcl, .terraformrc, and the _providers/ filesystem
# mirror.
#
# Terraform's mutable state (.terraform/, terraform.tfstate*) is
# created in the same dir; it survives until `bazel clean`. State
# proper lives in the GCS backend, so a clean only forces a re-init,
# not a re-apply.
#
# This script:
#   1. Resolves $WORK = dirname(rlocation(main.tf.json)).
#   2. Substitutes the @@MIRROR_PATH@@ placeholder in $WORK/.terraformrc
#      into a sibling $WORK/.terraformrc.runtime — leaves the bazel
#      output untouched so subsequent rebuilds don't churn.
#   3. Exports TF_CLI_CONFIG_FILE → the substituted file.
#   4. Stages tfvars / module-subdir files (from other packages) into
#      $WORK if any are declared.
#   5. Runs pre-apply hooks (apply only).
#   6. cd $WORK, terraform init, then the verb.
#
# Args:
#   $1  — terraform binary (already resolved to an absolute path)
#   $2  — verb: plan | apply | destroy
#   $3+ — extra flags forwarded to the terraform invocation
#
# Env (newline-separated rlocation paths; unset/empty means "none"):
#   TFRUNNER_GEN_FILES — generated `*.tf.json` + lockfile/.terraformrc/mirror
#   TFRUNNER_TFVARS    — `*.auto.tfvars.json` files
#   TFRUNNER_MODULES   — `<subdir>|<relpath>|<rloc>` triples (one per file)
#   TFRUNNER_PRE_APPLY — pre-apply executables (only run on `apply`)
#
# Env (control):
#   TF_AUTO_APPROVE   — if non-empty, pass `-auto-approve` to apply/destroy
#   TF_INIT_UPGRADE   — if non-empty, pass `-upgrade` to init (refresh lockfile)
#   TF_PLAN_JSON_OUT  — if set (plan only), save the binary plan via `-out=` and
#                       write `terraform show -json` output to this path. Used by
#                       `aspect plan` in CI to feed the structured PR-comment
#                       renderer. No effect on local interactive runs.
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
shift 2

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

# Resolve the workdir as the real bazel-bin directory of the
# generating tf_root. We DON'T use the runfiles tree here: under
# --nobuild_runfile_links (the workspace default) the runfiles dir is
# only a manifest, not a materialized tree, so terraform's
# filesystem_mirror would find an empty `_providers/` and fall back to
# the network. The bazel-bin dir, by contrast, is always materialized
# — symlinked via $BUILD_WORKSPACE_DIRECTORY/bazel-bin/.
if [[ -z "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
  echo "tf/run.sh: BUILD_WORKSPACE_DIRECTORY is unset; this script must be run via 'bazel run' (or an aspect-cli runnable that exports the var)" >&2
  exit 1
fi
if [[ -z "${TFRUNNER_WORKDIR_REL:-}" ]]; then
  echo "tf/run.sh: TFRUNNER_WORKDIR_REL not set by the wrapper template" >&2
  exit 1
fi
WORK="${BUILD_WORKSPACE_DIRECTORY}/bazel-bin/${TFRUNNER_WORKDIR_REL}"

# Stage tfvars and module subdir files into WORK. These can come from
# other packages, so they aren't necessarily already in $WORK. cp
# overwrites, which is what we want when an input has changed.
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

# Substitute @@MIRROR_PATH@@ in .terraformrc to the absolute workdir
# path. We write to a sibling `.terraformrc.runtime` rather than
# overwriting the bazel output (which would cause cache churn on
# subsequent builds). TF_CLI_CONFIG_FILE points terraform at the
# substituted file; the original .terraformrc is harmless because
# terraform doesn't auto-read it from cwd.
if [[ -f "$WORK/.terraformrc" ]]; then
  sed "s|@@MIRROR_PATH@@|$WORK|g" "$WORK/.terraformrc" > "$WORK/.terraformrc.runtime"
  export TF_CLI_CONFIG_FILE="$WORK/.terraformrc.runtime"
fi

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
    PLAN_ARGS=(-input=false "${NO_COLOR[@]}")
    if [[ -n "${TF_PLAN_JSON_OUT:-}" ]]; then
      # CI path: capture a binary plan so we can emit JSON for the
      # structured PR-comment renderer. We can't `exec` here — we still
      # need to run `terraform show -json` after plan returns.
      BIN_PLAN="$WORK/tfplan.bin"
      rm -f "$BIN_PLAN" "$TF_PLAN_JSON_OUT"
      PLAN_ARGS+=("-out=$BIN_PLAN")
      set +e
      "$TERRAFORM_BIN" plan "${PLAN_ARGS[@]}" "$@"
      RC=$?
      set -e
      if [[ $RC -eq 0 && -f "$BIN_PLAN" ]]; then
        # If `show -json` itself fails, drop the partial output so the
        # renderer falls back to its CAUTION-with-text-log path rather
        # than choking on a half-written JSON document.
        if ! "$TERRAFORM_BIN" show -json "$BIN_PLAN" > "$TF_PLAN_JSON_OUT"; then
          echo "WARN: terraform show -json failed; skipping structured plan" >&2
          rm -f "$TF_PLAN_JSON_OUT"
        fi
      fi
      exit "$RC"
    fi
    exec "$TERRAFORM_BIN" plan "${PLAN_ARGS[@]}" "$@"
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
