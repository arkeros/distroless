# OCI distribution

Bazel-defined OCI image build, supply-chain attestation, and distribution under the public `distroless.io` mirror.

## Language

### Distribution surfaces

**Public mirror surface** (a.k.a. "distroless.io"):
The set of images served at `distroless.io/<name>`, GHCR-backed at `ghcr.io/arkeros/senku/<name>`, fronted by the registry binary in `oci/cmd/registry`. Public, externally consumable. Every image on this surface is the published contract of the project — anything attached to it (signatures, SBOMs, attestations) is part of the contract consumers verify.
_Avoid_: "GHCR images" (ambiguous — not all GHCR-pushed images are mirror surface), "public images" (too broad).

**Internal deploy path**:
The GAR-hosted copies of selected images (`europe-docker.pkg.dev/senku-prod/containers/...`), pulled by Cloud Run for the registry's own runtime. Same digest as the public-mirror copy, different registry. Trust is IAM-bound (Cloud Run service account → GAR), not cosign-bound. Out of scope for cosign signing.
_Avoid_: "private mirror" (it's a deploy substrate, not a mirror), "GAR images" (the term overlaps non-deploy uses of GAR).

**Mirror image**:
An image published via `mirror_push` (`oci/mirror_push.bzl`). Definition is by-the-macro: if you `mirror_push`, it's a mirror image, signed and provenance-attested as a build-graph property. Pushing to the GHCR mirror surface (`ghcr.io/arkeros/senku/*`) without going through `mirror_push` is not allowed by convention; the only exception today is the registry binary's GAR copy, which uses plain `image_push` because GAR is the internal Cloud Run deploy substrate (IAM-trusted, not cosign-trusted).
_Avoid_: "signed image" (the signature is the consequence, not the identity); "GHCR image" (some GAR copies share digests but live on a different surface).

### Verification

**Verification perimeter** (for the public mirror):
External consumers running `cosign verify-attestation` on their own infrastructure. The project's role is to publish a stable identity policy (OIDC issuer + workflow subject), the signed digest, and the SLSA provenance attestation; consumers enforce by verifying the attestation matches the policy. No internal verify gate substitutes for this — internal Cloud Run uses IAM trust on the internal deploy path, which is a different perimeter.
_Avoid_: "Flux verify" (that's the internal-deploy concern, not the mirror concern), "`cosign verify`" without `-attestation` (the published attestation is what binds, not a bare signature alone).

#### Consumer verify command

```bash
cosign verify-attestation \
    --certificate-identity-regexp='^https://github\.com/arkeros/senku/\.github/workflows/ci\.yaml@refs/heads/main$' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --type=slsaprovenance \
    distroless.io/<image>:<tag>
```

The `--certificate-identity-regexp` pins the OIDC subject to `ci.yaml` on `main` of `arkeros/senku` — workflow-bound, not repo-bound. Off-policy signatures (different repo, different workflow file, different ref) are rejected. The exact strings are the single source of truth in [`//oci:cosign_policy.bzl`](../oci/cosign_policy.bzl); the producer `BUILDER_ID` and this regex are derived from the same constants.

### Build-graph artifacts

**`mirror_push`**:
The macro that defines a mirror image's full publication unit — wraps `image_push` + `cosign_sign` + `cosign_attest` + `slsa_predicate` so the policy ("every mirror image is signed and provenance-attested") is encoded in the build graph rather than in CI script discipline. There is no path to push a public-mirror image except through `mirror_push`.
_Avoid_: "image_push to GHCR" (that's the underlying primitive — `mirror_push` is the policy unit).

**SLSA provenance**:
The Bazel-built JSON predicate attached to each mirror image's digest, conforming to SLSA v1.0 ProvenanceStatement. Phase 1 captures `buildDefinition.externalParameters` (bazel target, source URI, git commit) + `runDetails.builder.id` (the workflow identity); phase 2 will add `resolvedDependencies` from the same `gather_metadata` aspect that drives SBOM generation. The OIDC certificate carries runtime fidelity (which workflow ran, when); the predicate carries build-graph fidelity (what was built, from what source).
_Avoid_: "GHA provenance" (the provenance is build-derived, not GHA-derived).
