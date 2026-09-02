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
   provenance together (it walks both referrers and legacy tags); `oras
   discover --format tree <ref>` shows only the two referrers. Every referrer's
   `artifactType` reads `application/vnd.oci.empty.v1+json`; the predicate
   type that tells an SBOM from provenance is inside the DSSE envelope, and
   only `cosign verify-attestation --type=…` and `slsa-verifier` read that
   deep.
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
| `publish` | yes: push by digest, sign, attest SBOM | Emits the pending digest list. A digest is pending when `slsa-verifier` says `no matching attestations`; any other verify error fails the job. |
| `provenance` | yes: attaches provenance | One generator call per pending digest. Skipped entirely when nothing is pending. |
| `verify` | **no** | `packages: read` only. Positive tests on every published digest, negative tests on two foreign images. |
| `release` | yes: tags | Runs every `_tag` target. Nothing here runs unless `verify` passed for every digest. |

A digest with no provenance is therefore always one of: the `provenance` job
has not run yet in this push, or it failed. Either way the next push to `main`
finds the digest pending and attests it. Nothing has to be repaired by hand.

## Debugging a failed verify

| Symptom | Likely cause | First check |
|---|---|---|
| `cosign`: `no matching attestations` | The SBOM attest step did not run for this digest | Re-run `bazel run <base>_attest_sbom` |
| `slsa-verifier`: `no matching attestations` | The `provenance` job has not reached this digest, or the matrix leg failed | Look at the `provenance` job in the same run; if it failed, the next push to `main` retries |
| `slsa-verifier`: `source used to generate the binary does not match provenance` | Provenance on this digest was produced for another repository | This is what the negative test asserts on `ghcr.io/kyverno/kyverno`. On one of our digests it means someone attached foreign provenance; start at Rekor. |
| `slsa-verifier`: `the image is mutable` | A tag was passed instead of a digest | `slsa-verifier` refuses tag references; resolve first with `crane digest` |
| `cosign`: `certificate verification failed: x509` | Stale TUF root or clock skew | `cosign initialize`; check system time |
| `cosign`: `none of the expected identities matched` | Signer subject does not match the regexp in `oci/cosign_policy.bzl` | Inspect the certificate SAN; if a workflow was renamed, the policy file is the one place to change |
| `transparency log entry not found` | Signed with `--tlog-upload=false` | We never do; check the invocation |
| Verify succeeds locally, fails in CI | Runner has no egress to `rekor.sigstore.dev` or `tuf-repo-cdn.sigstore.dev` | The `verify` job fetches the trusted root once with retries; look at that step |

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
