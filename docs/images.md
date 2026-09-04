# Distroless mirror images

Public mirror of distroless container images. Pull from `distroless.io/<image>:<tag>` — the canonical bytes live at `ghcr.io/arkeros/distroless/<image>:<tag>` and the `distroless.io` vanity domain is a proxy in front (see [`//oci/cmd/registry`](../oci/cmd/registry/README.md)). Both surfaces verify against the same identity policy; the examples below use `distroless.io` because that's the consumer-facing entry point.

Every image is published with signed artifacts from two signers:

| Artifact | Signed by | Inner predicate type | Verify with |
|---|---|---|---|
| Signature | this repository's `ci.yaml` on `main` | — (signature + cert only) | `cosign verify` |
| CycloneDX SBOM | same | `https://cyclonedx.org/bom` | `cosign verify-attestation --type=cyclonedx` |
| Vulnerability scan | same | `https://cosign.sigstore.dev/attestation/vuln/v1` | `cosign verify-attestation --type=vuln` |
| VEX document (images with statements) | same | `https://openvex.dev/ns` | `cosign verify-attestation --type=openvex` |
| SLSA provenance | `slsa-framework/slsa-github-generator` | `https://slsa.dev/provenance/v0.2` | `slsa-verifier verify-image` |

The signature, the SBOM, the scan and the VEX document are ours and are attached via the **OCI 1.1 referrers API** (the `subject` field of a separate manifest), stored as `application/vnd.dev.sigstore.bundle.v0.3+json` Sigstore bundles. `oras discover` shows them all with `artifactType` `application/vnd.oci.empty.v1+json`; the predicate type that tells them apart is inside the bundle's DSSE envelope, and `cosign verify-attestation --type=…` is what reads that deep.

The scan and the VEX document are deliberately two artifacts ([ADR 0015](./adr/0015-scan-and-vex-attestations.md)). The scan is [grype](https://github.com/anchore/grype)'s report on the SBOM, unfiltered, wrapped in cosign's [vulnerability scan record](https://github.com/sigstore/cosign/blob/main/specs/COSIGN_VULN_ATTESTATION_SPEC.md) so it names the scanner, the database build it consulted and when it ran; it says what the scanner found. The VEX document is this project's statement about those findings — which do not affect the image, and why — with a review-by date on every statement. Neither is rewritten in the light of the other, so each can be checked on its own, and a consumer joins them the way the directory at `distroless.io/directory/image/<image>/<tag>/vulnerabilities` does: a finding whose CVE a `not_affected` or `fixed` statement names is set aside, everything else stands. A scan is only as current as the database it names; a CVE published after that is not in it. The database pin is bumped daily and every `main` build re-attests any digest whose scans all name an older database (or the same database and an older grype), so a digest accumulates scans over its life and the newest is the one to read — `cosign verify-attestation` prints them all, one envelope per line. The VEX document is re-attested when its statements change, and the newest by log time is the one that speaks.

The provenance is different in two ways, both deliberate ([ADR 0014](./adr/0014-platform-provenance-slsa-github-generator.md)). It is written and signed by the generator's reusable workflow, which runs in its own VM after our build has finished, so nothing our build does can forge it — that is what makes it SLSA Build L3 rather than a statement the build made about itself. And because the generator attaches with its own cosign, it lands as the legacy `sha256-<hex>.att` sibling tag rather than as a referrer; `slsa-verifier` and cosign both find it there.

The generator's certificate is the same for every project on GitHub that uses it. What binds a provenance statement to *this* repository is the source URI inside the predicate, which is why provenance is verified with `slsa-verifier --source-uri` and never with a certificate identity alone.

Each artifact is bound to the image **digest**, not a tag. Consumers verify against the digest resolved from the tag they pull; `slsa-verifier` refuses tag references outright.

## Verify

The verification policy — our OIDC issuer and workflow subject, the generator's builder ID, and the source URI — is the single source of truth in [`//oci:cosign_policy.bzl`](../oci/cosign_policy.bzl). External consumers pin all of them so an artifact minted from a different repository or workflow is rejected.

Cosign 3.x discovers referrers by default and accepts no flag on `verify` to alter that — nothing to configure on the consumer side.

### Signature

```bash
cosign verify \
    --certificate-identity-regexp='^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    distroless.io/<image>:<tag>
```

### SLSA provenance

`slsa-verifier` checks the generator's signature, that the builder is the container generator at a `vX.Y.Z` tag, and that the provenance names this repository as its source:

```bash
DIGEST=$(crane digest distroless.io/<image>:<tag>)
slsa-verifier verify-image "distroless.io/<image>@${DIGEST}" \
    --source-uri github.com/arkeros/distroless \
    --builder-id https://github.com/slsa-framework/slsa-github-generator/.github/workflows/generator_container_slsa3.yml
```

Add `--source-tag` or `--source-branch main` to also pin the ref. `cosign verify-attestation --type=slsaprovenance` with the generator's identity regexp (`^https://github\.com/slsa-framework/slsa-github-generator/\.github/workflows/generator_container_slsa3\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`) verifies the signature but **not** the source; on its own it would accept any project's provenance on this digest.

### Vulnerability scan attestation

Verify only:

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=vuln \
    distroless.io/<image>:<tag>
```

Verify and extract grype's report, then list what stands after the VEX document is applied (the same join the directory does):

```bash
VERIFY=(cosign verify-attestation
    --certificate-identity-regexp='^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$'
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com')
"${VERIFY[@]}" --type=vuln distroless.io/<image>:<tag> 2>/dev/null \
  | jq -r '.payload' | base64 -d | jq '.predicate.scanner.result' > scan.grype.json
"${VERIFY[@]}" --type=openvex distroless.io/<image>:<tag> 2>/dev/null \
  | jq -r '.payload' | base64 -d | jq '.predicate' > vex.openvex.json   # absent for images with no statements
jq --slurpfile vex vex.openvex.json '
  [$vex[0].statements[] | select(.status == "not_affected" or .status == "fixed") | .vulnerability.name] as $silenced
  | [.matches[] | select(.vulnerability.id | IN($silenced[]) | not)
      | {id: .vulnerability.id, severity: .vulnerability.severity, package: .artifact.name, version: .artifact.version}]
  | unique' scan.grype.json
```

`grype sbom:sbom.cdx.json` against the SBOM download below reproduces the scan with today's database, which is the check to run when the record's `scanner.db.version` is older than you would like.

### CycloneDX SBOM attestation

Verify only:

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    distroless.io/<image>:<tag>
```

`cosign verify-attestation` prints the DSSE envelope on success. The actual CycloneDX BOM is base64-encoded inside `.payload` as an in-toto Statement; `.predicate` is what CycloneDX consumers want.

Verify and extract the BOM as raw CycloneDX JSON:

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    distroless.io/<image>:<tag> \
  | jq -r '.payload | @base64d | fromjson | .predicate' \
  > sbom.cdx.json
```

Verify and pipe straight into a vulnerability scanner (no temp file):

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    distroless.io/<image>:<tag> \
  | jq -r '.payload | @base64d | fromjson | .predicate' \
  | grype
```

Quick package summary:

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    distroless.io/<image>:<tag> \
  | jq -r '.payload | @base64d | fromjson | .predicate.components[]
           | "\(.name) \(.version)"'
```

Note that `cosign tree` and `oras discover` will *not* tell you which referrer is the SBOM — under cosign 3.x's bundle format every referrer is labelled `https://sigstore.dev/cosign/sign/v1` at the discovery layer, with the actual `predicateType` (`cyclonedx.org/bom`, `slsa.dev/provenance/v0.2`, etc.) one indirection deeper inside the DSSE envelope. `cosign verify-attestation --type=…` is the only built-in tool that reads that deep, which is why the recipes above all start there rather than picking a leaf digest by hand.

## Inspect referrers directly

Useful for debugging — what's actually attached to a digest:

```bash
# resolve digest first
DIGEST=$(crane digest distroless.io/<image>:<tag>)

# the image itself — no `subject` field, this is the signed thing
crane manifest "distroless.io/<image>@${DIGEST}" | jq

# enumerate referrers via the OCI 1.1 tag-fallback scheme
# (neither ghcr.io nor the distroless.io proxy in front of it serve
# /v2/<repo>/referrers/<digest> directly — both 303 to a 404. The
# spec mandates a `sha256-<hex>` tag pointing at an index of referrer
# manifests as the fallback, which is what cosign and oras consume.)
HEX="${DIGEST#sha256:}"
crane manifest "distroless.io/<image>:sha256-${HEX}" | jq
bazel run @land_oras_oras//cmd/oras -- discover --format tree \
    "distroless.io/<image>@${DIGEST}"
```

`oras discover` walks the referrers chain and prints a tree of the two Sigstore bundles we attach (signature, SBOM). The provenance is not a referrer — the generator attaches it as the `sha256-<hex>.att` tag, so `crane manifest "distroless.io/<image>:sha256-${HEX}.att" | jq` shows it. Their index `artifactType` is `application/vnd.oci.empty.v1+json` (the empty-config marker); the cosign-meaningful type lives inside each bundle's DSSE envelope and is what `cosign verify-attestation --type=...` keys off.

## Why OCI 1.1 referrers

The legacy cosign scheme (sibling tags `<digest>.sig` / `.att`) doesn't survive registry mirrors that don't replicate by tag pattern, conflicts with tag-immutability policies, and forces every consumer to know cosign's tag conventions. The OCI 1.1 referrers API is the spec-defined discovery path — `cosign verify`, `oras discover`, `crane`, and any spec-conformant registry tooling all find the artifacts the same way. The one exception is the provenance, whose layout is the generator's to choose, not ours. See [ADR 0006](./adr/0006-bazel-native-cosign-mirror-signing.md) and [ADR 0014](./adr/0014-platform-provenance-slsa-github-generator.md).

## See also

- [`//oci:cosign_policy.bzl`](../oci/cosign_policy.bzl) — single source of truth for the verify policy
- [`//oci:mirror_push.bzl`](../oci/mirror_push.bzl) — the build-graph policy unit
- [ADR 0014](./adr/0014-platform-provenance-slsa-github-generator.md) — platform provenance and the publish/verify/release pipeline
- [ADR 0015](./adr/0015-scan-and-vex-attestations.md) — the vulnerability scan and VEX attestations, how they are refreshed, and why they are two
- [ADR 0006](./adr/0006-bazel-native-cosign-mirror-signing.md) — Bazel-native cosign mirror signing (superseded; trust root and policy unit still apply)
- [Mirror signing runbook](./mirror-signing-runbook.md) — where artifacts live, debugging a failed verify
- [`CONTEXT.md`](../CONTEXT.md) — verification perimeter, and the vocabulary these docs use
- [OCI 1.1 referrers API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers)
