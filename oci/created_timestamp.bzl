"""RFC 3339 `created` timestamps for OCI image configs, derived from the
upstream-snapshot anchor in each distro's lockfile.

Background — see ADR 0007 §"Build horizon" amendment. Chainguard's
`build horizon` concept (https://edu.chainguard.dev/software-security/build-horizon/)
defines the maximum age an image is permitted to remain in production
before rebuild; admission controllers (e.g. sigstore/policy-controller's
`fetchConfigFile`) gate deployment on `image.config.created`. Without
a value, senku images are policy-invisible; with epoch 0, they appear
infinitely stale — the exact trap distroless PR #1203 fixed via
`SOURCE_DATE_EPOCH` from the source commit timestamp.

senku has a stronger signal than commit time: the upstream-snapshot
timestamp embedded in each distro's lockfile. It tracks dependency
freshness (which is what horizon admission actually cares about),
stays stable across same-lockfile rebuilds (Bazel caches cleanly),
and is symmetric across distros — Hummingbird pins via `repomd.xml`
revision (Unix epoch in `.repo.revision`), Debian pins via the
`snapshot.debian.org` archive timestamp (YYYYMMDDTHHMMSSZ embedded in
every package URL).

Each macro emits a one-file label suitable for `image_manifest(created = ...)`.
"""

load("@jq.bzl//jq:jq.bzl", "jq")

def hummingbird_created_timestamp(name, lock, **kwargs):
    """RFC 3339 timestamp derived from a Hummingbird (rules_rpm) lockfile.

    Reads `.repo.revision` — the Unix-epoch timestamp Hummingbird's
    `repomd.xml` carries (multi-update-per-day cadence; see ADR 0007
    §Snapshot strategy). `strftime` formats it as RFC 3339 UTC.

    Args:
        name: target name; emits `<name>.txt` consumable by `created = ":<name>"`.
        lock: label of an rpm.install lockfile (e.g. `//:hummingbird.lock.json`).
        **kwargs: forwarded to `jq` (e.g. `visibility`).
    """
    jq(
        name = name,
        srcs = [lock],
        out = name + ".txt",
        args = ["--raw-output", "--join-output"],
        filter = '.repo.revision | tonumber | strftime("%Y-%m-%dT%H:%M:%SZ")',
        **kwargs
    )

def debian_created_timestamp(name, lock, **kwargs):
    """RFC 3339 timestamp derived from a Debian (rules_distroless) lockfile.

    Extracts the `snapshot.debian.org` archive timestamp embedded in
    each package URL (path component `archive/debian/<YYYYMMDDTHHMMSSZ>/`).
    rules_distroless writes all URLs from a single snapshot resolve, so
    every package shares the same prefix — taking `packages[0].urls[0]`
    is sufficient. A drift across packages would mean the lockfile was
    hand-edited; safer to fail loud on that than to silently average.

    The format is parsed by string slicing rather than regex capture —
    the snapshot.debian.org URL shape is contractually fixed
    (https://snapshot.debian.org/), and explicit `[i:j]` slices fail
    loud on drift instead of papering over it.

    Args:
        name: target name; emits `<name>.txt` consumable by `created = ":<name>"`.
        lock: label of an apt.install lockfile (e.g. `//oci/distroless:debian.lock.json`).
        **kwargs: forwarded to `jq` (e.g. `visibility`).
    """
    jq(
        name = name,
        srcs = [lock],
        out = name + ".txt",
        args = ["--raw-output", "--join-output"],
        # urls[0] is the snapshot.debian.org primary; split("/")[5] is
        # the `YYYYMMDDTHHMMSSZ` segment. Position 8 is the literal `T`
        # date/time separator — skip it and pick hours from position 9.
        # String slices fail loud on drift (wrong length → garbage
        # output, easy to spot in CI) rather than papering over it.
        filter = (
            '.packages[0].urls[0] | split("/")[5] as $ts | ' +
            '"\\($ts[0:4])-\\($ts[4:6])-\\($ts[6:8])T\\($ts[9:11]):\\($ts[11:13]):\\($ts[13:15])Z"'
        ),
        **kwargs
    )
