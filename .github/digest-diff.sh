#!/usr/bin/env bash

# Which targets' bytes actually changed, as markdown on stdout, from two
# `knife bep digests` maps.
#
# The question target-determinator cannot answer. It reports what Bazel will
# re-run, which is what a test gate needs and is deliberately pessimistic: a
# change to a build tool — the `img` pusher every image layer goes through
# links half of go.mod — re-runs every action downstream of it and can still
# emit identical bytes. On the first live run that was the difference between
# "36 of 36 images affected" and the truth. Measured over //bazel/... for the
# same change: 8 targets affected, 5 whose outputs actually moved.
#
# The digests are Bazel's own. The digest function is sha256 (.bazelrc pins it
# so a layer's cache key is its registry digest), so this compares what the
# action cache already established rather than re-hashing anything.
#
# One caveat is load-bearing: the two maps come from two different runners, so
# a difference means "the bytes changed *or* they were never
# machine-independent". The second is a finding too — ci.yaml already rebuilds
# every image digest on a second runner and refuses to push when the two
# disagree, after run 33618108072 shipped a digest `publish` never pushed.
set -o errexit -o nounset -o pipefail

readonly BASELINE="${1:?usage: digest-diff.sh <baseline.json> <current.json>}"
readonly CURRENT="${2:?usage: digest-diff.sh <baseline.json> <current.json>}"

# Listed, not just counted, up to here. Past it the list has stopped being a
# summary and the counts are what get read anyway.
readonly LIST_LIMIT=25

DIFF="$(jq -n --slurpfile base "${BASELINE}" --slurpfile cur "${CURRENT}" '
    ($base[0] // {}) as $b | ($cur[0] // {}) as $c |
    {
      moved: [$c | to_entries[] | select($b[.key] and $b[.key] != .value) | .key] | sort,
      added: [$c | keys_unsorted[] | select($b[.] | not)] | sort,
      removed: [$b | keys_unsorted[] | select($c[.] | not)] | sort,
      total: ($c | length),
      baseline: ($b | length),
    }')"
readonly DIFF

get() { jq -r "$1" <<<"${DIFF}"; }

# A markdown list of a field, truncated, with the remainder counted.
listing() {
    local field="$1" heading="$2" count="$3"
    ((count > 0)) || return 0
    printf '\n%s\n\n' "${heading}"
    get ".${field}[0:${LIST_LIMIT}][] | \"- \`\(.)\`\""
    if ((count > LIST_LIMIT)); then
        printf '\n…and %d more.\n' "$((count - LIST_LIMIT))"
    fi
}

printf '## Outputs that actually changed\n\n'

if [[ "$(get '.total')" == "0" ]]; then
    printf 'Nothing was built, so there is nothing to compare.\n'
    exit 0
fi

# An empty baseline is not "every target is new"; it is no baseline.
if [[ "$(get '.baseline')" == "0" ]]; then
    printf "\`main\` has stored no digest map yet, so there is nothing to compare\n"
    printf "against. The first \`main\` build after this merges will leave one.\n"
    exit 0
fi

moved=$(get '.moved | length')
added=$(get '.added | length')
removed=$(get '.removed | length')

if ((moved == 0 && added == 0 && removed == 0)); then
    printf 'None. All %s targets that produced output produce the same bytes as\n' "$(get '.total')"
    printf "\`main\`: this change rebuilds, and changes nothing anyone consumes.\n"
    exit 0
fi

# Three counts rather than a fraction: a removed target is not in the set a
# fraction would be out of, so a single number would misstate one of them.
printf '%s changed, %s new, %s gone, of %s targets that produced output.\n' \
    "${moved}" "${added}" "${removed}" "$(get '.total')"

listing moved "### Different bytes than \`main\`" "${moved}"
listing added "### New — no output on \`main\` to compare" "${added}"
listing removed "### Gone — \`main\` produced output, this does not" "${removed}"
