#!/usr/bin/env bash
set -o pipefail -o errexit -o nounset

readonly COSIGN="{{cosign_path}}"
readonly DIGEST_FILE="{{digest_file}}"
readonly FIXED_ARGS=({{fixed_args}})

DIGEST=$(cat "${DIGEST_FILE}")
readonly DIGEST

# Compose fixed-args (from BUILD attrs) before runtime args from `bazel run -- ...`.
ALL_ARGS=(${FIXED_ARGS[@]+"${FIXED_ARGS[@]}"} "$@")
if [[ ${#ALL_ARGS[@]} -gt 0 ]]; then
  set -- "${ALL_ARGS[@]}"
fi

REPOSITORY=""
RECURSIVE=""
EXTRA_ARGS=()

while (( $# > 0 )); do
  case "$1" in
    --repository) shift; REPOSITORY="$1"; shift ;;
    --repository=*) REPOSITORY="${1#--repository=}"; shift ;;
    --recursive) RECURSIVE="--recursive"; shift ;;
    *) EXTRA_ARGS+=("$1"); shift ;;
  esac
done

if [[ -z "${REPOSITORY}" ]]; then
  echo "ERROR: --repository not set. Pass --repository=<registry>/<repo>, or set the 'repository' attribute on the rule." >&2
  exit 1
fi

# Key mode is runtime-controlled:
#   - COSIGN_KEY set    → key-based (KMS URI, file path) — emitted as `--key`.
#   - COSIGN_KEY unset  → keyless (Fulcio + Rekor); requires an OIDC token in env.
KEY_ARGS=()
if [[ -n "${COSIGN_KEY:-}" ]]; then
  KEY_ARGS+=("--key" "${COSIGN_KEY}")
fi

# `--yes` is always passed: rule is non-interactive (CI / automation).
exec "${COSIGN}" sign \
  --yes \
  ${RECURSIVE:+"${RECURSIVE}"} \
  ${KEY_ARGS[@]+"${KEY_ARGS[@]}"} \
  ${EXTRA_ARGS[@]+"${EXTRA_ARGS[@]}"} \
  "${REPOSITORY}@${DIGEST}"
