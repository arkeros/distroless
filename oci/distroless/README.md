# Distroless mirror images

Public mirror of distroless container images at `ghcr.io/arkeros/senku/*` (also reachable via the `distroless.io` vanity domain — see [`//oci/cmd/registry`](../cmd/registry/README.md)).

Every image is published with three signed artifacts, all attached via the **OCI 1.1 referrers API** (the `subject` field of a separate manifest, not legacy `.sig` / `.att` sibling tags):

| Artifact | Inner predicate type | Verify with |
|---|---|---|
| Signature | — (signature + cert only) | `cosign verify` |
| SLSA provenance | `https://slsa.dev/provenance/v0.2` | `cosign verify-attestation --type=slsaprovenance` |
| CycloneDX SBOM | `https://cyclonedx.org/bom` | `cosign verify-attestation --type=cyclonedx` |

All three are stored on the registry as `application/vnd.dev.sigstore.bundle.v0.3+json` Sigstore bundles (cosign 3.x's default `--new-bundle-format`), so they're uniform on the wire — `oras discover` will show every entry's `artifactType` as `application/vnd.oci.empty.v1+json` (the index-entry marker) regardless of which artifact it is. The discrimination happens inside the bundle: cosign decodes it and inspects the inner DSSE envelope's predicate type.

Each is bound to the image **digest**, not a tag. Consumers verify against the digest cosign resolves from the tag they pull.

## Verify

The verification policy (OIDC issuer + workflow subject) is the single source of truth in [`//oci:cosign_policy.bzl`](../cosign_policy.bzl). External consumers pin both `--certificate-oidc-issuer` and `--certificate-identity-regexp` so a signature minted from a different repo or workflow file is rejected.

Cosign 3.x discovers referrers by default and accepts no flag on `verify` to alter that — nothing to configure on the consumer side.

### Signature

```bash
cosign verify \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    ghcr.io/arkeros/senku/<image>:<tag>
```

### SLSA provenance attestation

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=slsaprovenance \
    ghcr.io/arkeros/senku/<image>:<tag>
```

### CycloneDX SBOM attestation

Verify only:

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    ghcr.io/arkeros/senku/<image>:<tag>
```

`cosign verify-attestation` prints the DSSE envelope on success. The actual CycloneDX BOM is base64-encoded inside `.payload` as an in-toto Statement; `.predicate` is what CycloneDX consumers want.

Verify and extract the BOM as raw CycloneDX JSON:

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    ghcr.io/arkeros/senku/<image>:<tag> \
  | jq -r '.payload | @base64d | fromjson | .predicate' \
  > sbom.cdx.json
```

Verify and pipe straight into a vulnerability scanner (no temp file):

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    ghcr.io/arkeros/senku/<image>:<tag> \
  | jq -r '.payload | @base64d | fromjson | .predicate' \
  | grype sbom:-
```

Quick package summary:

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    ghcr.io/arkeros/senku/<image>:<tag> \
  | jq -r '.payload | @base64d | fromjson | .predicate.components[]
           | "\(.name) \(.version)"'
```

Note that `cosign tree` and `oras discover` will *not* tell you which referrer is the SBOM — under cosign 3.x's bundle format every referrer is labelled `https://sigstore.dev/cosign/sign/v1` at the discovery layer, with the actual `predicateType` (`cyclonedx.org/bom`, `slsa.dev/provenance/v0.2`, etc.) one indirection deeper inside the DSSE envelope. `cosign verify-attestation --type=…` is the only built-in tool that reads that deep, which is why the recipes above all start there rather than picking a leaf digest by hand.

## Inspect referrers directly

Useful for debugging — what's actually attached to a digest:

```bash
# resolve digest first
DIGEST=$(crane digest ghcr.io/arkeros/senku/<image>:<tag>)

# the image itself — no `subject` field, this is the signed thing
crane manifest "ghcr.io/arkeros/senku/<image>@${DIGEST}" | jq

# enumerate referrers via the OCI 1.1 tag-fallback scheme
# (ghcr.io doesn't serve /v2/<repo>/referrers/<digest> directly;
# the spec mandates a `sha256-<hex>` tag pointing at an index of
# referrer manifests, which is what cosign and oras both consume)
HEX="${DIGEST#sha256:}"
crane manifest "ghcr.io/arkeros/senku/<image>:sha256-${HEX}" | jq
bazel run @land_oras_oras//cmd/oras -- discover --format tree \
    "ghcr.io/arkeros/senku/<image>@${DIGEST}"
```

`oras discover` walks the referrers chain and prints a tree of the three attached Sigstore bundles. Their index `artifactType` is `application/vnd.oci.empty.v1+json` (the empty-config marker); the cosign-meaningful type lives inside each bundle's DSSE envelope and is what `cosign verify-attestation --type=...` keys off.

## Why OCI 1.1 referrers

The legacy cosign scheme (sibling tags `<digest>.sig` / `.att`) doesn't survive registry mirrors that don't replicate by tag pattern, conflicts with tag-immutability policies, and forces every consumer to know cosign's tag conventions. The OCI 1.1 referrers API is the spec-defined discovery path — `cosign verify`, `oras discover`, `crane`, and any spec-conformant registry tooling all find the artifacts the same way. See [ADR 0006](../../docs/adr/0006-bazel-native-cosign-mirror-signing.md) for the broader signing rationale.

## See also

- [`//oci:cosign_policy.bzl`](../cosign_policy.bzl) — single source of truth for the verify policy
- [`//oci:mirror_push.bzl`](../mirror_push.bzl) — the build-graph policy unit
- [ADR 0006](../../docs/adr/0006-bazel-native-cosign-mirror-signing.md) — Bazel-native cosign mirror signing
- [`docs/oci-CONTEXT.md`](../../docs/oci-CONTEXT.md) — verification perimeter, threat model
- [OCI 1.1 referrers API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers)
