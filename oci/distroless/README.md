# Distroless mirror images

Public mirror of distroless container images at `ghcr.io/arkeros/senku/*` (also reachable via the `distroless.io` vanity domain — see [`//oci/cmd/registry`](../cmd/registry/README.md)).

Every image is published with three signed artifacts, all attached via the **OCI 1.1 referrers API** (the `subject` field of a separate manifest, not legacy `.sig` / `.att` sibling tags):

| Artifact | Type | Predicate type |
|---|---|---|
| Signature | `application/vnd.dev.cosign.simplesigning.v1+json` | — |
| SLSA provenance | DSSE-wrapped in-toto attestation | `slsaprovenance` (SLSA v1.0) |
| CycloneDX SBOM | DSSE-wrapped in-toto attestation | `cyclonedx` |

Each is bound to the image **digest**, not a tag. Consumers verify against the digest cosign resolves from the tag they pull.

## Verify

The verification policy (OIDC issuer + workflow subject) is the single source of truth in [`//oci:cosign_policy.bzl`](../cosign_policy.bzl). External consumers pin both `--certificate-oidc-issuer` and `--certificate-identity-regexp` so a signature minted from a different repo or workflow file is rejected.

Cosign 2.x+ discovers OCI 1.1 referrers by default — no extra flag needed. If you're behind a registry proxy that doesn't yet support `/referrers/`, pass `--registry-referrers-mode=oci-1-1` to force the spec'd discovery path. (The `distroless.io` proxy supports it.)

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

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=cyclonedx \
    ghcr.io/arkeros/senku/<image>:<tag>
```

`cosign verify-attestation` prints the DSSE envelope on success; pipe through `jq -r '.payload | @base64d | fromjson | .predicate'` to extract the SBOM itself.

## Inspect referrers directly

Useful for debugging — what's actually attached to a digest, by media type:

```bash
# resolve digest first
DIGEST=$(crane digest ghcr.io/arkeros/senku/<image>:<tag>)

# enumerate referrers
crane manifest "ghcr.io/arkeros/senku/<image>@${DIGEST}" | jq    # the image itself, has no `subject`
oras discover --format tree "ghcr.io/arkeros/senku/<image>@${DIGEST}"
```

`oras discover` walks the referrers chain and prints a tree of attached signatures + attestations grouped by `artifactType`.

You can also hit the registry endpoint directly:

```bash
curl -sSL -H "Accept: application/vnd.oci.image.index.v1+json" \
    "https://ghcr.io/v2/arkeros/senku/<image>/referrers/${DIGEST}" | jq
```

## Why OCI 1.1 referrers

The legacy cosign scheme (sibling tags `<digest>.sig` / `.att`) doesn't survive registry mirrors that don't replicate by tag pattern, conflicts with tag-immutability policies, and forces every consumer to know cosign's tag conventions. The OCI 1.1 referrers API is the spec-defined discovery path — `cosign verify`, `oras discover`, `crane`, and any spec-conformant registry tooling all find the artifacts the same way. See [ADR 0006](../../docs/adr/0006-bazel-native-cosign-mirror-signing.md) for the broader signing rationale.

## See also

- [`//oci:cosign_policy.bzl`](../cosign_policy.bzl) — single source of truth for the verify policy
- [`//oci:mirror_push.bzl`](../mirror_push.bzl) — the build-graph policy unit
- [ADR 0006](../../docs/adr/0006-bazel-native-cosign-mirror-signing.md) — Bazel-native cosign mirror signing
- [`docs/oci-CONTEXT.md`](../../docs/oci-CONTEXT.md) — verification perimeter, threat model
- [OCI 1.1 referrers API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers)
