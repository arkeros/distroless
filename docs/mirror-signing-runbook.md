# Mirror signing runbook

Operational notes for the three artifacts every **Mirror** image carries. The
decisions are in [ADR 0014](./adr/0014-platform-provenance-slsa-github-generator.md)
and, for what it kept, [ADR 0006](./adr/0006-bazel-native-cosign-mirror-signing.md);
the consumer-facing verify commands are in [`images.md`](./images.md). This
file is what you open when one of them fails.

## What is attached to a digest, and by whom

| Artifact | Signer identity | Written by | Verify with |
|---|---|---|---|
| Signature | `…/arkeros/distroless/.github/workflows/ci.yaml@refs/heads/main` | `mirror_push`'s `_sign` target, run by the `publish` job | `cosign verify` |
| **SBOM** (CycloneDX) | same | `mirror_push`'s `_attest_sbom` target, same job | `cosign verify-attestation --type=cyclonedx` |
| **Vulnerability scan** (cosign vuln record around grype's report) | same | `mirror_push`'s `_attest_vuln` target, same job | `cosign verify-attestation --type=vuln` |
| **VEX document** (OpenVEX; only images with statements) | same | `mirror_push`'s `_attest_vex` target, same job | `cosign verify-attestation --type=openvex` |
| **Platform provenance** (SLSA v0.2) | `…/slsa-framework/slsa-github-generator/.github/workflows/generator_container_slsa3.yml@refs/tags/vX.Y.Z` | the generator's reusable workflow, `provenance` job | `slsa-verifier verify-image --source-uri github.com/arkeros/distroless --builder-id …` |

The provenance signer is the generator's, not ours. It is the same for every
project on GitHub that uses the generator, which is why verifying it means
checking `--source-uri` inside the predicate and never just the certificate.

The signature and the SBOM are Sigstore bundles attached over the OCI 1.1
referrers API — a subject-bearing manifest discoverable through
`GET /v2/<repo>/referrers/<digest>` or, on registries that do not serve that
endpoint, through the spec-mandated `sha256-<hex>` fallback tag. The
`sha256-<hex>` tags `crane ls` shows are those fallback indices, and deleting
them breaks discovery for clients that use the fallback.

The provenance is attached by the generator's own cosign, and lands as the
legacy `sha256-<hex>.att` sibling tag, not as a referrer. That layout is the
generator's to choose; `slsa-verifier` and `cosign verify-attestation` both
look there. Deleting a `.att` tag deletes the provenance.

## Where a signature event is recorded

1. **The registry.** `cosign tree <ref>` enumerates signature, SBOM and
   provenance together: it walks OCI referrers and legacy `.att`/`.sig` tags
   alike, and labels each referrer with the predicate type it found inside
   the bundle (checked with cosign 3.1.3 against both layouts). `oras
   discover --format tree <ref>` shows only the referrers, all with
   `artifactType` `application/vnd.oci.empty.v1+json`, because it does not
   open the bundle. Digests first published before September 2026 carry
   duplicates: `cosign attest` appends a new referrer on every run rather
   than replacing the last one, and `publish` used to re-sign every digest on
   every push. It now signs and attests only where ours is missing.
   Verification accepts any of the duplicates.
2. **Rekor**, the public transparency log. Every keyless signature lands there.
   Search by digest at <https://search.sigstore.dev/> or:

   ```sh
   curl -fsSL https://rekor.sigstore.dev/api/v1/index/retrieve \
     -X POST -H 'Content-Type: application/json' \
     -d '{"hash": "sha256:<hex>"}'
   ```

   The entry carries the Fulcio certificate, so the OIDC subject and issuer
   can be audited, and the inclusion proof.
3. **GitHub Actions.** The certificate's `OIDCBuildConfigUri` extension carries
   the run URL, so a Rekor entry cross-references to the exact CI run — ours
   for signature and SBOM, the generator's invocation from our run for
   provenance.
4. **GHCR package activity** logs pushes. There is no API for it; correlate
   with push-to-`main` events.

## The publish pipeline

`ci.yaml` on push to `main`: `test` → `publish` → `provenance` → `verify` →
`release`, with `push-gar` and `deploy-*` beside them. What each job may do:

| Job | Mutates the registry? | Notes |
|---|---|---|
| `publish` | yes: push by digest, then sign and attest the SBOM **only where ours is missing** | Verifies each of the three artifacts per digest first. Absent or someone else's → add (or, for provenance, list as pending); present and ours → skip; any other verify error fails the job. |
| `provenance` | yes: attaches provenance | One generator call per pending digest. Skipped entirely when nothing is pending. |
| `verify` | **no** | `packages: read` only. Refuses an empty list, then positive tests on every published digest and negative tests on two foreign images. |
| `release` | yes: tags | `crane tag` over the refs and tags `publish` reported and `verify` checked; no Bazel. Nothing here runs unless `verify` passed for every digest. |

A digest with no provenance is therefore always one of: the `provenance` job
has not run yet in this push, or it failed. Either way the next push to `main`
finds the digest pending and attests it. Nothing has to be repaired by hand.

`test` records every image's digest as it built it, and `publish` refuses to
push an image whose digest differs on its own runner. That failure names the
image and both digests; it means some input to the image varies per machine.
The last case was an mtree entry without `time=`, which the tar.bzl
reproducibility validator in `.bazelrc` now rejects at build time.

## Debugging a failed verify

| Symptom | Likely cause | First check |
|---|---|---|
| `cosign`: `no matching attestations`, or `none of the attestations matched the predicate type: cyclonedx` | The SBOM attest step did not run for this digest (the second wording appears when a signature referrer exists but no SBOM) | Re-run `bazel run <base>_attest_sbom`; in `publish` both are classified as absent and trigger it |
| `cosign`: `none of the attestations matched the predicate type: vuln` (or `openvex`) | The scan (or VEX) attest step did not run for this digest. Digests published before scans were attached look like this until `publish` next runs against them, which it does on every `main` build | Re-run `bazel run <base>_attest_vuln` (or `_attest_vex`); `publish` classifies it as absent and triggers it. The Directory's vulnerabilities page for the digest is a 404 until then |
| Several `vuln` (or `openvex`) attestations on one digest | Expected. `publish` adds a scan whenever the pinned grype database is newer than any scan on the digest, and a VEX document whenever its statements changed. The Directory shows the newest scan by `scanFinishedOn` and the newest VEX document by log time | Nothing. To prune, delete older referrers via the packages API (see GHCR notes); never the newest |
| `slsa-verifier`: `no matching attestations` | The `provenance` job has not reached this digest, or the matrix leg failed | Look at the `provenance` job in the same run; if it failed, the next push to `main` retries |
| `slsa-verifier`: `source used to generate the binary does not match provenance` | Provenance on this digest was produced for another repository | This is what the negative test asserts on `ghcr.io/kyverno/kyverno`. On one of our digests it means someone attached foreign provenance; start at Rekor. |
| `slsa-verifier`: `the image is mutable` | A tag was passed instead of a digest | `slsa-verifier` refuses tag references; resolve first with `crane digest` |
| `cosign`: `certificate verification failed: x509` | Stale TUF root or clock skew | `cosign initialize`; check system time |
| `cosign`: `none of the expected identities matched` | Signer subject does not match the regexp in `oci/cosign_policy.bzl` | Inspect the certificate SAN; if a workflow was renamed, the policy file is the one place to change |
| `transparency log entry not found` | Signed with `--tlog-upload=false` | We never do; check the invocation |
| Verify succeeds locally, fails in CI | Runner has no egress to `rekor.sigstore.dev`, or (slsa-verifier only) to `tuf-repo-cdn.sigstore.dev` | cosign in CI uses the pinned root from `//oci:sigstore_trusted_root` and makes no TUF request; slsa-verifier still does |
| `http_file` checksum mismatch for `sigstore_trusted_root` in CI | Sigstore published a new trusted root | Bump the pin in `bazel/include/oci.MODULE.bazel` per its comment; this fixes CI and the Directory together |

## When something looks wrong

Start at Rekor. Every keyless signature attempt lands there, whether or not a
consumer would accept it. A Rekor entry for one of our digests whose Fulcio
subject is neither `ci.yaml@refs/heads/main` nor the generator workflow is
someone minting signatures consumers will reject — not an exposure, but worth
finding out who.

## Rotation

There is no key to rotate. A workflow rename or runner migration changes the
OIDC subject of the signature and the SBOM; a generator bump changes nothing
consumers pin, because they pin the workflow path and a `v[0-9]+.[0-9]+.[0-9]+`
tag pattern.

1. Edit `oci/cosign_policy.bzl`. The CI env file, the negative tests and the
   docs derive from it. The Directory's `web/internal/policy` duplicates the
   signer constants and must move with it.
2. Do not delete old signatures. They stay valid against the old identity for
   anyone who pinned it; new images get the new identity.
3. Record the cutover date in `images.md`.

## Generator bumps

Renovate opens the pull request; it is excluded from auto-merge. Check the
generator's release notes for a changed `buildType` or predicate version, since
`slsa-verifier` and the generator move together, and bump `slsa-verifier` in
`tools/tools.lock.json` in the same change when the release notes say to.
