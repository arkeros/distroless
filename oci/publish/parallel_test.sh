#!/usr/bin/env bash
# Drives parallel.sh with stub commands and asserts the four properties the
# `publish` job depends on once its loop stops being serial:
#
#   1. every item runs, and each item's output is contiguous and in input
#      order — concurrent workers must not shred the `::group::` blocks;
#   2. an item that fails does not stop the others, and is named at the end;
#   3. a worker that dies without recording a status counts as a failure,
#      never as a pass — "refusing to guess" must survive the move into a
#      subprocess (ADR 0014);
#   4. the items actually run concurrently.
#
# Nothing here touches a registry: the "work" is a stub script per case.
set -o errexit -o nounset -o pipefail

WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

FAILURES=0

fail() {
    echo "FAIL: $*" >&2
    FAILURES=$((FAILURES + 1))
}

# ---------------------------------------------------------------------------
# 1. Output is contiguous per item and replayed in input order.
#
# The stub writes three lines with pauses between them, so a driver that let
# workers write straight to stdout would interleave them.
# ---------------------------------------------------------------------------
cat >"${WORK}/emit" <<'EOF'
#!/usr/bin/env bash
printf 'begin %s\n' "$1"
sleep 0.2
printf 'middle %s\n' "$1"
sleep 0.2
printf 'end %s\n' "$1"
EOF
chmod +x "${WORK}/emit"

printf '%s\n' alpha bravo charlie delta >"${WORK}/items.txt"

cat >"${WORK}/expected_order.txt" <<'EOF'
begin alpha
middle alpha
end alpha
begin bravo
middle bravo
end bravo
begin charlie
middle charlie
end charlie
begin delta
middle delta
end delta
EOF

if ! "${PARALLEL_SH}" -P 4 -- "${WORK}/emit" \
    <"${WORK}/items.txt" >"${WORK}/order.out" 2>"${WORK}/order.err"; then
    fail "parallel.sh exited non-zero when every item succeeded:"
    cat "${WORK}/order.err" >&2
fi

if ! diff -u "${WORK}/expected_order.txt" "${WORK}/order.out"; then
    fail "output was interleaved or out of input order (see diff above)."
fi

# ---------------------------------------------------------------------------
# 2. One failing item fails the run, is named, and does not stop the rest.
# ---------------------------------------------------------------------------
cat >"${WORK}/sometimes" <<'EOF'
#!/usr/bin/env bash
printf 'working on %s\n' "$1"
if [[ "$1" == "bad" ]]; then
    echo "refusing to guess" >&2
    exit 1
fi
EOF
chmod +x "${WORK}/sometimes"

printf '%s\n' good-one bad good-two >"${WORK}/mixed.txt"

if "${PARALLEL_SH}" -P 4 -- "${WORK}/sometimes" \
    <"${WORK}/mixed.txt" >"${WORK}/mixed.out" 2>"${WORK}/mixed.err"; then
    fail "parallel.sh exited zero although an item failed; a failed publish must fail the job."
fi

if ! grep -q "bad" "${WORK}/mixed.err"; then
    fail "the failure summary does not name the item that failed:"
    cat "${WORK}/mixed.err" >&2
fi

for ITEM in good-one good-two; do
    if ! grep -q "working on ${ITEM}" "${WORK}/mixed.out"; then
        fail "item ${ITEM} did not run; one failing item must not stop the others."
    fi
done

# The failing item's own output still has to reach the log — it is the only
# record of why it failed.
if ! grep -q "refusing to guess" "${WORK}/mixed.out" "${WORK}/mixed.err"; then
    fail "the failing item's output was swallowed."
fi

# ---------------------------------------------------------------------------
# 3. A worker killed outright is a failure, not a pass.
#
# The stub kills its own parent — the worker — so no exit status is ever
# recorded for the item. That is what an OOM kill looks like, and the run
# must not report success.
# ---------------------------------------------------------------------------
cat >"${WORK}/suicide" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "doomed" ]]; then
    kill -KILL "${PPID}"
    sleep 5
fi
printf 'survived %s\n' "$1"
EOF
chmod +x "${WORK}/suicide"

printf '%s\n' healthy doomed >"${WORK}/doomed.txt"

if "${PARALLEL_SH}" -P 2 -- "${WORK}/suicide" \
    <"${WORK}/doomed.txt" >"${WORK}/doomed.out" 2>"${WORK}/doomed.err"; then
    fail "a worker that died without recording a status was reported as success."
fi

if ! grep -q "doomed" "${WORK}/doomed.err"; then
    fail "the killed item was not named in the failure summary:"
    cat "${WORK}/doomed.err" >&2
fi

# ---------------------------------------------------------------------------
# 4. The items really do run at the same time.
#
# Eight one-second items at -P 8 finish in about a second; serially they take
# eight. The bound is loose so a slow machine does not fail the build.
# ---------------------------------------------------------------------------
cat >"${WORK}/slow" <<'EOF'
#!/usr/bin/env bash
sleep 1
printf 'done %s\n' "$1"
EOF
chmod +x "${WORK}/slow"

seq 1 8 >"${WORK}/eight.txt"

START=${SECONDS}
"${PARALLEL_SH}" -P 8 -- "${WORK}/slow" <"${WORK}/eight.txt" >/dev/null 2>&1
ELAPSED=$((SECONDS - START))
if [[ "${ELAPSED}" -ge 5 ]]; then
    fail "eight one-second items took ${ELAPSED}s at -P 8; they did not run concurrently."
fi

# ---------------------------------------------------------------------------
if [[ "${FAILURES}" -ne 0 ]]; then
    echo "${FAILURES} assertion(s) failed." >&2
    exit 1
fi
echo "OK: parallel.sh keeps output contiguous, ordered, and fails loudly."
