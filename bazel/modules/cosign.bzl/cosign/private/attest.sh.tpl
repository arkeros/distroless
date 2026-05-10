#!/usr/bin/env bash
set -o pipefail -o errexit -o nounset

readonly COSIGN="{{cosign_path}}"
readonly DIGEST_FILE="{{digest_file}}"
readonly FIXED_ARGS=({{fixed_args}})

DIGEST=$(cat "${DIGEST_FILE}")
readonly DIGEST

ALL_ARGS=(${FIXED_ARGS[@]+"${FIXED_ARGS[@]}"} "$@")
if [[ ${#ALL_ARGS[@]} -gt 0 ]]; then
  set -- "${ALL_ARGS[@]}"
fi

REPOSITORY=""
PREDICATE=""
TYPE=""
REFERRERS_MODE=""
EXTRA_ARGS=()

while (( $# > 0 )); do
  case "$1" in
    --repository) shift; REPOSITORY="$1"; shift ;;
    --repository=*) REPOSITORY="${1#--repository=}"; shift ;;
    --predicate) shift; PREDICATE="$1"; shift ;;
    --predicate=*) PREDICATE="${1#--predicate=}"; shift ;;
    --type) shift; TYPE="$1"; shift ;;
    --type=*) TYPE="${1#--type=}"; shift ;;
    --registry-referrers-mode) shift; REFERRERS_MODE="$1"; shift ;;
    --registry-referrers-mode=*) REFERRERS_MODE="${1#--registry-referrers-mode=}"; shift ;;
    *) EXTRA_ARGS+=("$1"); shift ;;
  esac
done

if [[ -z "${REPOSITORY}" ]]; then
  echo "ERROR: --repository not set. Pass --repository=<registry>/<repo>, or set the 'repository' attribute on the rule." >&2
  exit 1
fi
if [[ -z "${PREDICATE}" ]]; then
  echo "ERROR: --predicate not set." >&2
  exit 1
fi
if [[ -z "${TYPE}" ]]; then
  echo "ERROR: --type not set." >&2
  exit 1
fi

KEY_ARGS=()
if [[ -n "${COSIGN_KEY:-}" ]]; then
  KEY_ARGS+=("--key" "${COSIGN_KEY}")
fi

exec "${COSIGN}" attest \
  --yes \
  --type "${TYPE}" \
  --predicate "${PREDICATE}" \
  ${REFERRERS_MODE:+--registry-referrers-mode "${REFERRERS_MODE}"} \
  ${KEY_ARGS[@]+"${KEY_ARGS[@]}"} \
  ${EXTRA_ARGS[@]+"${EXTRA_ARGS[@]}"} \
  "${REPOSITORY}@${DIGEST}"
