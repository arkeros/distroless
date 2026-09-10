#!/usr/bin/env bash

# What a pull request actually changes, as Bazel sees it.
#
# `git diff` is a poor answer for this repo: the changes that are hardest to
# read are the ones with the smallest diffs. A Renovate bump of an *indirect*
# Go module is three lines in go.mod/go.sum and moves the bytes of every Go
# binary we ship, and no diff of the checkout says so.
#
# target-determinator answers it by analysing the graph at both revisions and
# comparing configured targets, so a change that only reaches the build
# through an external repository is still counted. bazel-diff was measured
# against this repo first and cannot see that case at all: under bzlmod it
# models external repos as synthetic `//external:<apparent name>` labels, and
# an indirect Go module never gets an apparent name, so nothing it hashes
# changes. Its documented workaround — seeding on go.sum — marks 4062 of 4135
# targets impacted. Zero or everything; nothing in between.
#
# The raw list is not the useful part. `rdeps(//..., <the module this pull
# request bumps>)` reaches 568 first-party targets, and reading 568 labels is
# no better than reading the diff. What is useful is which of the sets we
# already care about it lands in — the images we publish, and the tests. That
# roll-up is the reason this is a script and not a `run:` block.
set -o errexit -o nounset -o pipefail

# `comm` compares byte by byte and trusts its inputs to be sorted the way it
# would sort them; one locale for every `sort` here keeps that true.
export LC_ALL=C

readonly BASE="${1:?usage: affected-targets.sh <base-ref>}"
readonly TARGET_DETERMINATOR="${TARGET_DETERMINATOR:-target-determinator}"
readonly BAZEL="${BAZEL:-bazel}"

# The merge base, not the base branch tip: a target that main changed under us
# is not something this pull request did.
MERGE_BASE="$(git merge-base HEAD "${BASE}")"
readonly MERGE_BASE

WORK="$(mktemp -d)"
readonly WORK
trap 'rm -rf "${WORK}"' EXIT

# `-before-query-error-behavior=fatal`, not the default. The default is
# `ignore-and-build-all`: when the 'before' query fails, target-determinator
# reports *every* target as affected and exits 0. A silent "everything
# changed" is worse than no answer, because nobody re-reads a summary that
# always says the same thing.
#
# The query is `cquery`, so this only works where the whole graph analyses.
# On a macOS host four targets fail (`//oci/cmd/registry` and
# `//web/cmd/server` have no darwin base image), and they are reached through
# `deps()` whatever the pattern is, so scoping does not rescue it and
# `--keep_going` only re-enters the fallback above. On the Linux runners the
# host is the target and it analyses clean.
"${TARGET_DETERMINATOR}" \
    -bazel "${BAZEL}" \
    -before-query-error-behavior=fatal \
    -targets '//...' \
    "${MERGE_BASE}" |
    # target-determinator abbreviates `//pkg/name:name` to `//pkg/name`;
    # `bazel query --output=label` never does. Expand so the two can be
    # compared with `comm`, which needs both sides spelled the same way.
    sed -E 's|^//(.*/)?([^:/]+)$|//\1\2:\2|' |
    sort -u >"${WORK}/affected"

query() {
    "${BAZEL}" query --noshow_progress --output=label "$1" 2>/dev/null | sort -u
}

# The images the mirror publishes, by the same query ci.yaml uses to record
# their digests, so this summary and the digests that get signed are talking
# about one set.
query 'attr(tags, mirror_push_managed, kind(image_push, //...))' >"${WORK}/images"
query 'tests(//...)' >"${WORK}/tests"
# Rules, not `//...`: that would count every source file in every package,
# while target-determinator reports configured targets and nothing else.
query 'kind(rule, //...)' >"${WORK}/rules"

comm -12 "${WORK}/affected" "${WORK}/images" >"${WORK}/affected-images"
comm -12 "${WORK}/affected" "${WORK}/tests" >"${WORK}/affected-tests"

count() { wc -l <"$1" | tr -d ' '; }

cat <<EOF
## Affected targets

Compared against \`${MERGE_BASE:0:12}\`, the merge base with \`${BASE}\`.

| Set | Affected | Total |
| --- | -------: | ----: |
| Published images | $(count "${WORK}/affected-images") | $(count "${WORK}/images") |
| Tests | $(count "${WORK}/affected-tests") | $(count "${WORK}/tests") |
| All targets | $(count "${WORK}/affected") | $(count "${WORK}/rules") |
EOF

# One markdown bullet per label. Double quotes with the backticks escaped: a
# `sed 's|$|`|'` would be shorter, but both the `$` anchor and the backticks
# read as unexpanded expressions to shellcheck, and .shellcheckrc is empty on
# purpose — the defaults are the policy, so the code moves rather than the
# rule.
bullets() {
    local label
    while IFS= read -r label; do
        printf -- "- \`%s\`\n" "${label}"
    done
}

# Listed in full: thirty-six is short enough to read, and "which published
# images move" is the question this whole job exists to answer.
if [[ -s "${WORK}/affected-images" ]]; then
    printf '\n### Published images that change\n\n'
    bullets <"${WORK}/affected-images"
else
    printf '\nNo published image changes.\n'
fi

# Capped: an indirect dependency bump reaches hundreds of tests, and a summary
# nobody scrolls to the end of has failed at being a summary.
readonly TEST_LIMIT=40
if [[ -s "${WORK}/affected-tests" ]]; then
    printf '\n### Tests that change\n\n'
    head -n "${TEST_LIMIT}" "${WORK}/affected-tests" | bullets
    remaining=$(($(count "${WORK}/affected-tests") - TEST_LIMIT))
    if ((remaining > 0)); then
        printf '\n…and %d more.\n' "${remaining}"
    fi
fi
