# A ledger of tag moves, per family, on the registry

**Status:** Accepted, 2026-09-06.

Each family on the **Mirror** keeps a **Tag ledger**: an append-only record
of which **Digest** each of its tags was pointed at, and when. The `release`
job appends to it right after applying the tags, and only for tags that
moved. It is an OCI artifact of JSON Lines at
`<registry>/<prefix>/history:<family>`, next to the family's own repository.
It exists so the **Directory** can show a tag's vulnerability history: the
scans on every digest the tag has named, stitched together in order.

## Why now

ADR 0015 made a digest accumulate scan records over its life, one per move of
the vulnerability database pin, so every digest already carries a time series
of what a scanner found in it. Nothing joined those series to a tag. Which
build `nginx:latest` named last week is registry metadata that the registry
does not keep: `release` moves every tag of a family to the new build, the
old digest keeps no tag, and the distribution API lists tags only. Once a tag
has moved, the digest it left cannot be enumerated, let alone dated.

The scans are on the registry and signed. The gap is one small fact per tag
move, and the only place that fact is known is the job that makes the move.

## Decisions

### An event log of moves, not a daily sample

One line per move: the time, the tag, the digest, and the URL of the run that
applied it. Not one line per day per tag, which is what a chart shows but not
what happened. The scan records supply the daily cadence on their own; the
ledger only has to say which digest's records apply between two moments. It
is smaller, exact to the minute, and it says the same thing a reader sees on
the versions page, just with a date on it.

The family is not repeated per line: the artifact hangs on the family's tag
in the `history` repository. The run URL is not validated and nothing decides
on it; it is the one link from an event back to the job.

### In the registry, as an OCI artifact

The ledger lives where everything else the Directory reads lives, behind the
same auth the Directory already carries, and is read the same way an
attestation is: resolve a tag, fetch the one layer. It is an OCI 1.1 artifact
with `application/vnd.distroless.tag-ledger.v1` as its artifact type, the
empty descriptor as config, and one JSON Lines layer. Every push replaces the
whole artifact, so its digest changes on every event and the Directory can
cache a parsed ledger by that digest, exactly as it caches a digest's
components. Old versions of the ledger stay on GHCR untagged, which is a
history of the history until someone prunes it.

A database was considered and put off. The writer is one serial job, the
reader wants one range read per family, and the volume is a few lines per
family per week. That is the shape of a file. A Firestore or Cloud SQL
instance would add a durable resource to `//infra`, two identity grants, a
client library in the Directory, an emulator for its tests, and — for
Postgres — an instance billed around the clock behind a service that scales
to zero. It would also make the binding a row asserted by whoever held write
access, where an artifact on the registry can be signed by the release's own
identity if that is ever wanted. The trigger to revisit is a query across
families, a second writer, or a ledger too large to fetch per page view.

A file committed to the repository was the other option, and it has the
better append: atomic, and audited by git. It was passed over because a CI
commit to `main` triggers another run, and because the Directory would only
see the file on its next deploy rather than when the tag moved.

### The rule is a tested jq module; the plumbing is `oras`

`release` applies every tag on every run, so "applied" is not "moved" and the
difference is a rule: an applied tag is a move when the ledger's last event
for that tag names a different digest, by file order rather than by the
event's own time, which two releases in a second could share. It lives in
`//oci/publish:ledger.jq` with a table-driven test, as `stale.jq` does and for
the same reason: it decides what is written to a public registry, and it
refuses rather than guesses when the ledger or the applied list is not what
it claims to be. A rule that answered "nothing moved" to a ledger it could
not read would lose history silently; one that answered "everything moved"
would fill it with events that are not moves.

The pull and push are `oras`, pinned in `tools/tools.lock.json` with the other
CI binaries, so the version has one home and `bazel run @multitool//tools/oras`
is the same binary. `release` installs it by that URL and checksum through its
GitHub action rather than by version: the action only knows the releases it
shipped with, and the first pin was one it had not heard of. The job runs
without Bazel, as `crane` does. A ledger that does not exist yet is
empty; a ledger that cannot be read for any other reason stops the job,
because appending to "empty" would write over it.

### One release at a time

Read, append, write back: two releases interleaving would drop the earlier
one's events. The `release` job holds a concurrency group with nothing
cancelled, since a release that has applied its tags must get to record them.

## Consequences

- A tag's history starts at the first release after this ADR. Nothing
  reconstructs what came before; the digests are gone from the listing.
- The ledger is the same trust class as a tag: registry metadata, signed by
  nobody, tamper-evident by content addressing. The counts a chart draws from
  it are read off verified scan records, and the ledger only says which
  record is the tag's on which day. Signing the artifact is one `cosign sign`
  in `release` if a signed binding is ever wanted; the Directory would then
  need a signature check alongside its attestation check.
- ADR 0015's pruning policy said older scans may be deleted because nothing
  reads them. A tag chart reads them. Scans inside whatever window the
  Directory draws must stay; older ones remain prunable.
- The `history` package is created by the first push and GHCR makes new
  packages private. It has to be made public once, by hand, like the image
  packages were, or the Directory's read will fail with a 404 that says
  nothing about visibility.

## Revisit when

- The Directory needs a query the ledger cannot answer with one fetch per
  family, e.g. a front page ranking every image by open findings today.
- Something other than `release` needs to write it.
