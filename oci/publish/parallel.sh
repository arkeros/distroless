#!/usr/bin/env bash
# Runs one command per input line, several at a time, without losing either
# the output or the exit status of any of them.
#
#   parallel.sh -P <n> -- <command> [args...]   < items
#
# Each line of stdin is appended to <command> as its last argument. Up to <n>
# run at once; the run fails, naming every item that failed, if any of them
# did.
#
# It exists because `publish` and `verify` spend their time waiting on GHCR,
# Fulcio and Rekor — around eight round trips per image, ~25 s of latency and
# almost no CPU — so the useful width is set by what those services tolerate,
# not by the runner's cores. Two properties make this worth a script rather
# than a bare `&`:
#
#   Output is buffered per item and replayed in input order. A `::group::`
#   block is thousands of bytes written over many syscalls; workers sharing a
#   terminal shred each other's.
#
#   A worker that fails — or dies without recording anything — fails the run.
#   The callers' "refusing to guess" paths (ADR 0014: one signature and one
#   attestation of each kind per digest) abort with a non-zero exit, and in a
#   subprocess that only ends the subprocess. Silently skipping such an item
#   would let `release` tag a digest nothing had proven.
#
# Failures are collected rather than stopping the run, for the reason the
# `release` job gives for the same choice: the items are independent, a
# stopped run leaves the earlier ones already done, and the caller learns more
# from every failure than from the first one.
set -o errexit -o nounset -o pipefail

# ---------------------------------------------------------------------------
# Worker mode: one re-exec of this script per item, spawned by the `xargs`
# below. It always exits 0 itself — a non-zero worker makes xargs abandon the
# items it has not started, and every item must get its turn — so the real
# status goes to a file that the parent reads afterwards.
# ---------------------------------------------------------------------------
if [[ "${1:-}" == "--worker" ]]; then
    shift
    INDEXED=$1
    shift
    INDEX=${INDEXED%%"${PARALLEL_SEPARATOR}"*}
    ITEM=${INDEXED#*"${PARALLEL_SEPARATOR}"}
    if "$@" "${ITEM}" >"${PARALLEL_LOG_DIR}/${INDEX}.log" 2>&1; then
        printf '0' >"${PARALLEL_LOG_DIR}/${INDEX}.status"
    else
        printf '%s' "$?" >"${PARALLEL_LOG_DIR}/${INDEX}.status"
    fi
    exit 0
fi

JOBS=""
while (($# > 0)); do
    case "$1" in
    -P)
        shift
        JOBS=$1
        shift
        ;;
    -P*)
        JOBS=${1#-P}
        shift
        ;;
    --)
        shift
        break
        ;;
    *)
        echo "ERROR: unknown option '$1'; usage: parallel.sh -P <n> -- <command> [args...]" >&2
        exit 2
        ;;
    esac
done

if [[ -z "${JOBS}" ]]; then
    echo "ERROR: -P <n> is required." >&2
    exit 2
fi
if (($# == 0)); then
    echo "ERROR: no command after '--'." >&2
    exit 2
fi

# The script re-executes itself as the worker, so it needs its own path even
# when it was invoked through a relative one. `realpath` is not on every
# runner; this is.
SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")

WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT
export PARALLEL_LOG_DIR="${WORK}"

# ASCII unit separator, not a tab: `xargs -I` hands the whole line over as one
# argument, and a separator it could read as whitespace is one BSD/GNU
# difference away from splitting an item in half.
export PARALLEL_SEPARATOR=$'\037'

# Number the items, so the logs replay in the order the items arrived whatever
# order the workers finish in, and so two identical items cannot share a log.
awk -v sep="${PARALLEL_SEPARATOR}" 'NF { printf "%06d%s%s\n", ++n, sep, $0 }' >"${WORK}/items"

COUNT=$(wc -l <"${WORK}/items" | tr -d ' ')
if [[ "${COUNT}" -eq 0 ]]; then
    echo "ERROR: no items on stdin; running nothing is not a success." >&2
    exit 1
fi

# Not `set -e`-fatal: whatever xargs made of the run, the logs below are the
# record of it and the per-item statuses are what decides the exit code.
XARGS_STATUS=0
xargs -I{} -P "${JOBS}" "${SELF}" --worker {} "$@" <"${WORK}/items" || XARGS_STATUS=$?

: >"${WORK}/failed"
while IFS= read -r INDEXED; do
    INDEX=${INDEXED%%"${PARALLEL_SEPARATOR}"*}
    ITEM=${INDEXED#*"${PARALLEL_SEPARATOR}"}
    if [[ -f "${WORK}/${INDEX}.log" ]]; then
        cat "${WORK}/${INDEX}.log"
    fi
    STATUS=$(cat "${WORK}/${INDEX}.status" 2>/dev/null || true)
    case "${STATUS}" in
    0) ;;
    # No status at all: the worker was killed before it could record one
    # (an OOM kill looks exactly like this). Not knowing how an item ended
    # is not the same as it having succeeded.
    "") printf '%s (no exit status recorded — the worker did not finish)\n' "${ITEM}" >>"${WORK}/failed" ;;
    *) printf '%s (exit %s)\n' "${ITEM}" "${STATUS}" >>"${WORK}/failed" ;;
    esac
done <"${WORK}/items"

if [[ -s "${WORK}/failed" ]]; then
    {
        echo "ERROR: $(wc -l <"${WORK}/failed" | tr -d ' ') of ${COUNT} item(s) failed:"
        sed 's/^/  /' "${WORK}/failed"
    } >&2
    exit 1
fi

if [[ "${XARGS_STATUS}" -ne 0 ]]; then
    echo "ERROR: every item reported success but xargs exited ${XARGS_STATUS}; refusing to call that a pass." >&2
    exit 1
fi
