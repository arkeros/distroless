# Vulnerability scan and VEX as separate, refreshed attestations

**Status:** Accepted, 2026-09-04.

Every image on the **Mirror** carries two attestations beyond the **SBOM** and
**Platform provenance**: a **Scan record** — grype's unfiltered report on the
**Index**'s SBOM, wrapped in cosign's vulnerability scan predicate — and, for
images with **VEX statement**s, one OpenVEX document. They are attested apart,
joined by the consumer, and the scan is re-attested whenever the pinned
vulnerability database or the scanner moves, so a **Digest** accumulates scans
over its life and the newest is the one to read. One scan per index both
decides publication and is that record. The **Directory** renders it at
`/directory/image/<family>/<ref>/vulnerabilities`.

## Why now

The Directory shows what is inside an image from evidence it verifies first.
What a scanner found in it is the next thing a reader asks, and nothing on the
mirror said. The gates ran grype on every build and threw the report away once
it had passed; a consumer wanting the answer had to download the SBOM and run
their own scanner. That is a fine thing to be able to do and a poor thing to
have to do, and it left the project's own verdicts — the VEX statements it
publishes on the record — unreadable from the mirror.

The alternative of scanning inside the Directory at request time was rejected
outright: it would put a vulnerability database in a service whose only
promise is that it renders nothing it did not verify, and would show a result
nobody signed. Every page of the Directory is a projection of an attestation.
This one is too.

## Decisions

### Two attestations, not one

The scan is attested as grype wrote it — matches, database, version — inside
cosign's [vulnerability scan record](https://github.com/sigstore/cosign/blob/main/specs/COSIGN_VULN_ATTESTATION_SPEC.md),
predicate type `https://cosign.sigstore.dev/attestation/vuln/v1`. The VEX
documents that gate the same image are merged into one OpenVEX document and
attested as `--type=openvex`. Neither is rewritten in the light of the other.

The obvious alternative is one attestation: run grype with `--vex` so
suppressed matches land in `ignoredMatches` with their status, and attest
that. It was tried. On an `sbom:` source grype applies statements scoped to a
package (`pkg:deb/debian/nginx`) and silently ignores statements scoped to the
image (`pkg:oci/nginx`), because an SBOM scan has no OCI identifier to match
them against. Half the statements would have vanished with no error. The
gates already avoided this by joining on CVE identifier in a jq filter
(`grype.bzl`), and the Directory does the same: a finding whose CVE a
`not_affected` or `fixed` statement names is shown set aside, everything else
stands. `affected` and `under_investigation` are statements too, and silence
nothing.

Keeping them apart also keeps each one what it claims to be. `cosign
verify-attestation --type=vuln` yields a report the scanner would recognise as
its own; `--type=openvex` yields the project's statements and nothing else. A
consumer can check either without the other, and join them any way they like.

### One scan per index, which is the gate

The publication gates used to run on each architecture's manifest, eight per
matrix (two architectures, two users, two modes). The attested record needed a
report over the Index, so the first version of this work added a ninth scan
per matrix over the index SBOM. That was two computations of one truth: the
index SBOM is the same generator's walk over the same graph as the per-arch
SBOMs, with every purl carrying its architecture, so a scan of it is every
per-arch scan in one report.

The gates therefore moved to the index. `oci_image` grew a `gate` switch that
`distroless_matrix` turns off for the per-arch manifests, and
`image_supply_chain` runs once per index with the same `fail_on_severity`,
`ignore_cves` and `vex` the arch images used to get; debug indexes add the
debug suppressions as before. Fewer scans than before this work began, and a
property worth more than the compute: **the report a consumer verifies is the
report that decided publication**. Before, they agreed by construction, and
"by construction" is a claim about the builder, which is the kind of claim
this project exists to stop making. Findings still name their architecture,
since the purl does. The two application images already worked this way,
their `image_from_binary` being multi-platform with one `image_supply_chain`;
the matrix was the outlier.

### Refresh: re-attest when the database or the scanner moves

A scan is a statement about a moment, and its moment is the database build it
names. The database pin (`//bazel/include:oci.MODULE.bazel`) is bumped daily.
On every `main` build, `publish` compares the scan it just built against every
scan already on the digest, ranked as `[database build time, scanner version]`,
and attests the new one if it outranks them all. The scan's own timestamp is
deliberately not consulted: every run has a new one, and a rule on it would
re-attest on every build. The VEX document is re-attested when its statement
set differs from every one attached — a statement added, withdrawn,
re-justified, or given a new expiry, since a re-justification is a new
statement in this project's model. Signatures and SBOMs stay one per digest.

The rule is a jq module with a table-driven test, `//oci/publish:stale.jq`,
not inline YAML: it decides whether to write to a public registry, and a rule
that cannot be run against a fixture cannot be trusted with that. It is jq
because the comparison is jq — CI only adds the plumbing that feeds it what
the registry carries. "Cannot tell" is a failure of the rule rather than a
third verdict, and CI aborts on it, because a policy that answers "attest" to
a malformed input adds referrers until someone notices, and one that answers
"skip" hides a broken pipeline behind a green job.

Consequently a digest carries a history: every scan it ever had, and every
VEX document. The Directory shows the newest scan by its `scanFinishedOn` and
the newest VEX document by the transparency log's time, which the verifier now
exposes as `SignedAt`. The log's clock is the one date about an attestation
its author did not write, and it is what makes a withdrawn statement stop
silencing: the older document is still valid, still verifiable, and no longer
the one that speaks. Scans are cached in the Directory for an hour, not for
the life of the process.

### Pruning: newest of each kind is the contract

Daily re-attestation adds roughly one referrer per image per day. GHCR does
not implement the registry `DELETE` for referrers; they go through the
packages API, and referrer count is what drives `cosign verify` time on GHCR.
The policy is fixed here so the mechanism can follow when needed:

- A digest keeps its newest scan record and its newest VEX document. Anything
  older may be deleted at any time; nothing reads it, and the Directory and
  the verify recipes in `docs/images.md` are written to the newest.
- Signatures, SBOM and provenance are never pruned.
- The trigger to automate pruning is `cosign verify` on a mirror image taking
  visibly longer than it does today, or a digest passing a few dozen
  referrers, whichever comes first. Until then the count is a number to watch,
  not a problem to solve.

## Consequences

- A build published before this ADR has no scan until `publish` next runs
  against its digest, which happens on every `main` build; its vulnerabilities
  page is a 404 that says so in the meantime. An empty table would read as a
  clean result, which "not scanned" is the opposite of.
- A scan is only as current as the database it names, and the page says which
  one. A CVE published after that build is not on the page until the next
  refresh, about a day.
- Wontfix `ignore_cves` entries have no signed record — they are the project
  accepting a finding permanently, not a statement about it — so those
  findings show as open. That is honest, and it is a reason to write a VEX
  statement instead where one can be justified.
- The gate names moved from `<family>_<mode>_<user>_<arch>_<distro>_cve_test`
  to `<family>_<mode>_<user>_<distro>_cve_test`. Nothing referenced them by
  name.
- The `stale` rule assumes grype writes the database build time as RFC 3339
  in UTC, which it does; the rule refuses to rank anything else rather than
  compare it as a string, and the test pins that.

## Revisit when

- grype's `--vex` learns to match image-scoped products on an SBOM source. The
  two-attestation shape would still be right for the reasons above, but the
  argument against the one-attestation shape would be weaker.
- A consumer needs the per-architecture gates back, e.g. to publish one
  architecture of a build whose other architecture fails. Nothing today does.
- Referrer growth trips the pruning trigger.
