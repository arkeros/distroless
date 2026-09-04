# Whether the attestation CI is about to add supersedes the ones a digest
# already carries. See ADR 0015.
#
# Entry point: `stale($type; $local)`, with the predicates already attached
# (of that type, as `cosign verify-attestation` yields them, unwrapped) as the
# input array, the cosign predicate type as $type, and the predicate about to
# be attested as $local. Yields "stale" when nothing attached is as good as
# $local and it should be attested, "fresh" when something attached is at
# least as good and the digest should be left alone — or fails, when $local is
# not what $type says it is, or $type has no rule. The caller must stop on a
# failure, not guess: a policy that answers "attest" to a malformed input adds
# referrers until someone notices, and one that answers "skip" hides a broken
# pipeline behind a green job.
#
# From CI, with the module on the search path:
#
#   attested vuln "$REF" \
#     | jq -r -s -L oci/publish --arg type vuln --slurpfile local "$PREDICATE" \
#         'include "stale"; stale($type; $local[0])'

# A scan ranks by the vulnerability database's build time, then the scanner's
# version, as [epoch, [major, minor, patch]] — arrays compare element-wise. A
# newer database knows more CVEs; a newer scanner matches differently. The
# scan's own timestamp is deliberately not consulted: every run has a new one,
# and a rule on it would re-attest every run.
#
# fromdateiso8601 accepts exactly the RFC 3339 UTC form grype writes, and fails
# on anything else — an offset, a fraction — rather than letting a string
# compare order two spellings of one instant.
def vuln_rank:
  [
    (.scanner.db.version | fromdateiso8601),
    (.scanner.version | split(".") | map(tonumber))
  ];

# Every field of every statement counts, so a re-justification or a bumped
# expiry is a change; order does not.
def vex_canonical:
  .statements | map(tojson) | sort;

# The local record must rank, or there is nothing to compare. An attached one
# that will not rank counts as older than anything: it should be superseded,
# not protected.
def stale_vuln($local):
  ($local | try vuln_rank catch error("not a vulnerability scan record: need scanner.db.version as RFC 3339 UTC and scanner.version as a dotted number")) as $rank
  | map(try vuln_rank catch [0])
  | all(. < $rank);

def stale_openvex($local):
  ($local | try vex_canonical catch error("not an OpenVEX document: no statements list")) as $mine
  | map(try vex_canonical catch null)
  | all(. != $mine);

def stale($type; $local):
  (
    if $type == "vuln" then stale_vuln($local)
    elif $type == "openvex" then stale_openvex($local)
    # One SBOM per digest, always.
    elif $type == "cyclonedx" then false
    else error("no staleness rule for predicate type \($type)")
    end
  )
  | if . then "stale" else "fresh" end;
