#!/usr/bin/env bash
# Generated wrapper. Bakes in the bucket name and the runfiles locations of
# the built webroot and its cache rules, so `bazel run //apps/x:bucket_push`
# takes no arguments — and so a direct spawn (`aspect apply` running this as
# a deploy node) behaves identically, which `bazel run`-only args injection
# would not.
#
# Path resolution goes through Bazel's bash runfiles library, so this script
# works whether the runfiles symlink tree was materialized
# (`--build_runfile_links`) or only the manifest is on disk
# (`--nobuild_runfile_links`, the workspace default). Probing for a
# `$0.runfiles` *directory* is not good enough: under the workspace default
# it never exists, and the wrapper fails before doing anything.
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

resolve() {
  local path
  path="$(rlocation "$1")" || true
  if [[ -z "$path" || ! -e "$path" ]]; then
    echo >&2 "bucket-push: cannot resolve $1 in runfiles"
    exit 2
  fi
  echo "$path"
}

PUBLISH_BIN="$(resolve '{PUBLISH_BIN}')"
WEBROOT="$(resolve '{WEBROOT}')"
CACHE_RULES="$(resolve '{CACHE_RULES}')"

# The publisher reads no runfiles of its own, but export these anyway so a
# child process that does finds the same source of truth rather than
# re-discovering it through `$0.runfiles` fallbacks that are absent here.
runfiles_export_envvars

exec "$PUBLISH_BIN" \
  -bucket '{BUCKET}' \
  -webroot "$WEBROOT" \
  -rules "$CACHE_RULES" \
  "$@"
