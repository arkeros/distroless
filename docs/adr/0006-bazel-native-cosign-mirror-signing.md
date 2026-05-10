# Bazel-native cosign signing for the distroless.io public mirror

Mirror images — anything published under the `distroless.io` brand, GHCR-hosted at `ghcr.io/arkeros/senku/*` — are signed and SLSA-provenance-attested by Bazel-native rules. Composition is via a `mirror_push` macro (`oci/mirror_push.bzl`) that bundles `image_push` + `cosign_sign` + `cosign_attest` + `slsa_predicate` into one policy unit, so no public-mirror push is reachable without its sign+attest siblings — the build graph encodes the policy.

## Enforcement

The "every mirror image is signed and attested" claim is enforced at Bazel analysis time by `mirror_push_enforcement_aspect` (`oci/aspects.bzl`), wired in `.bazelrc` via `common --aspects=...` so it runs on every `bazel build`, `bazel run`, and `bazel test`. The aspect inspects every `image_push` rule's `registry` and `repository` attrs; if they fall under the mirror prefix (`ghcr.io/arkeros/senku/*`) and the rule is missing the `mirror_push_managed` tag, analysis fails with an error pointing at this ADR. The `mirror_push` macro tags every `image_push` it generates with `mirror_push_managed`, so legitimate uses pass; raw uses do not.

The tag is a *convention*, not a Bazel-enforceable security boundary — macros are erased at analysis time, so there's no way to distinguish "tag set by `mirror_push`" from "tag added manually by a malicious PR." The actual defense against deliberate bypass is PR review: a `tags = ["mirror_push_managed"]` literal on a raw `image_push` is conspicuous, and CODEOWNERS on `oci/mirror_push.bzl` and `oci/aspects.bzl` is the human-review gate. The aspect's value is catching *accidental* misuse (a contributor who didn't know the convention) and forcing deliberate bypass to be explicit and reviewable.

## Threat model

| Capability | Defended? | Notes |
|---|---|---|
| Compromised registry (GHCR substitutes a malicious image at our path/digest) | **Yes** | The Sigstore signature binds publisher identity to the digest. Substituted images don't have a `cosign verify-attestation` for our OIDC subject. |
| Compromised pull (MITM between consumer and registry) | **Yes** | Same — the substituted image lacks our signature. |
| Compromised CI workflow file (PR adds a new `.github/workflows/foo.yml` that calls cosign sign) | **Yes** | The verify regex is workflow-bound (`ci\.yaml@refs/heads/main`), not repo-bound. A new workflow file on `main` would not mint signatures consumers accept. CODEOWNERS on `.github/workflows/ci.yaml` is the human-review gate that prevents the workflow file itself being modified. |
| Compromised non-`main` branch (PR branch tries to sign) | **Yes** | The verify regex pins `refs/heads/main`. `pull_request` events run with `id-token: read` (not `write`); they cannot mint OIDC tokens regardless. |
| Compromised consumer verify command (loose regex, missing issuer flag) | No (consumer responsibility) | We publish the canonical command; consumers who deviate are on their own. The CI negative test asserts our own command is strict. |
| Compromised GitHub Actions runner (attacker steals OIDC token mid-run, signs a malicious image with valid identity) | **No** | A runner-level compromise can mint signatures with our identity for the duration of the run. Mitigation is outside this design's scope (runner hardening, workload attestation chain). Detection is via Rekor: every signature lands in the transparency log; off-policy publish events are auditable. |
| Compromised Sigstore root (Fulcio CA, Rekor log integrity) | **No** | Sigstore's trust roots are the foundation we sit on. Mitigation is outside this design's scope; defenders rely on Sigstore's own threat model and TUF root rotation. |
| Compromised Bazel build (malicious in-tree rule writes a lying predicate) | Partially | The signing rules and predicate generator are in-tree (`bazel/modules/cosign.bzl/`, `oci/mirror_push.bzl`, `oci/cosign_policy.bzl`) and reviewed via CODEOWNERS. A reviewer-bypassing change would have to land in one of those files — defended by repo branch protection on `main`, not by signing itself. |
| Pinned-tag confusion (consumer pulls `latest`, attacker substitutes a different `latest`) | **Yes** | Signatures are digest-bound. `cosign verify-attestation` resolves the tag, fetches the manifest, and verifies the resulting digest. |
| Replay (older valid signature reused on a stale digest) | **Yes** | Signature binds to digest; stale digest means stale image content. Consumer policies that pin specific tags or digests reject staleness. |
| In-tree code adds a raw `image_push` to the mirror prefix bypassing `mirror_push` | **Yes** (analysis-time) | `mirror_push_enforcement_aspect` fails Bazel analysis. See *Enforcement* above; the tag-vs-PR-review nuance applies — accidental bypass is blocked, deliberate bypass becomes a reviewable change. |
| Compromised cosign binary download (`sigstore/cosign` GitHub release tarball substituted) | **Yes** | Per-platform SHA256 pinned in `bazel/modules/cosign.bzl/cosign/versions.bzl`. A substituted release fails the checksum and Bazel refuses to materialize the toolchain. |

## Trust root

Keyless GitHub Actions OIDC (Fulcio + Rekor), subject pinned to `https://github.com/arkeros/senku/.github/workflows/ci.yaml@refs/heads/main`. KMS support is parameterized via the `COSIGN_KEY` env var so a future migration to BuildBuddy / Tekton / Cloud Build / Codeberg is a runner change, not a Bazel rule change.

| Option | Verdict |
|---|---|
| **Keyless OIDC + Fulcio + Rekor** | **Chosen.** No long-lived secret to rotate; identity = workflow file path + ref + repo, reviewable in PRs via CODEOWNERS; Rekor gives transparency-log detection for free. |
| GCP KMS | Rejected. Long-lived secret, human in the rotation path, no transparency log without explicit opt-in, and weaker for nothing in return given the threat model (registry compromise / mirror integrity). |
| Static keypair | Rejected. Same drawbacks as KMS plus a secret-distribution problem. |

**Producer–consumer asymmetry on runner migration.** The `COSIGN_KEY` env var makes the *producer* (the signing rules) runner-agnostic — switching from GitHub Actions to BuildBuddy / Tekton / Cloud Build / Codeberg is a CI change, not a Bazel rule change. The *consumer* policy (`CERTIFICATE_IDENTITY_REGEXP`, `CERTIFICATE_OIDC_ISSUER` in `oci/cosign_policy.bzl`) is single-runner: it pins one OIDC issuer and one workflow path. A runner migration breaks every consumer's verify command overnight unless we either (a) add the new issuer/subject to the consumer policy in coordination with the migration, or (b) dual-sign during the transition window. This asymmetry is acceptable today because there are no external consumers to coordinate with; revisit before adding any. The choice trades consumer-side flexibility for producer-side simplicity, which is the right priority while we're still building the consumer surface.

## Identity policy

The OIDC subject pin is workflow-bound on `main` (`…/ci.yaml@refs/heads/main`).

| Option | Verdict |
|---|---|
| **Workflow file on `main`** | **Chosen.** CODEOWNERS on the workflow file is the human review; the OIDC subject string is the perimeter pinned in consumer verify policies. |
| Repo-bound (`…/.+@.*`) | Rejected. A malicious PR can add a new workflow file in `.github/workflows/` and mint a signature with that identity. |
| Tag-bound (`…@refs/tags/v*`) | Rejected. The repo's tags (`weekly_tag.yaml`) are calendar checkpoints, not release events; tag-bound would add zero semantic value over `main` and create a window where a fresh main HEAD has no signing authority. |

## Scope

Every image pushed to the mirror surface goes through `mirror_push` (currently: `oci/distroless/{bash,nginx}`, `oci/cmd/registry`, `devtools/workstation`). The GAR copy of `oci/cmd/registry` is **out of scope** — it's the Cloud Run deploy substrate, IAM-trusted, same digest as the GHCR copy but a different surface.

## Composition shape

A new `mirror_push` macro that wraps push + sign + attest + predicate.

| Option | Verdict |
|---|---|
| **`mirror_push` macro** | **Chosen.** Names the policy unit: there is no public-mirror push without sign+attest siblings, by construction. `bazel query "kind(mirror_push, //...)"` enumerates the entire mirror surface. |
| Flag on `image_supply_chain` (`mirror = True`) | Rejected. Conflates hygiene (SBOM/CVE — orthogonal, applies to every image) with distribution (sign/attest — applies only to mirror images), and the supply-chain macro doesn't carry registry/repo info. |
| Three sibling rules called by hand | Rejected. Loses the policy-by-construction property: a future PR could add an `image_push` to the mirror prefix and forget the sign target, going out unsigned. |

## Module location

`bazel/modules/cosign.bzl/` — a Bazel module rooted in the monorepo, shape-matched to `grype.bzl` but unpublished. Senku-specific wiring (workspace status keys, build-type URI, workflow identity) is parameterized as rule attributes.

| Option | Verdict |
|---|---|
| **`bazel/modules/cosign.bzl/` (in-monorepo module)** | **Chosen.** Module discipline enforced by `MODULE.bazel` boundary (the module can't reach into senku-private code); externalization later is `git subtree split` + bump `bazel_dep` line. No publishing scaffolding upfront. |
| Sibling repo (`github.com/arkeros/cosign.bzl`) from day one | Rejected. Premature externalization: no second consumer today, scaffolding cost (multitool lockfile, README, examples) for no immediate benefit. |
| Flat in-tree (`oci/cosign/`) | Rejected. No module boundary; the reusable rules would inevitably accrete senku-specific imports, and externalizing later means a real refactor instead of a `git subtree split`. |

## Cosign delivery

Loaded as a prebuilt binary via a module extension (`cosign/extensions.bzl`), with per-platform URLs and SHA256 checksums pinned in `versions.bzl`.

| Option | Verdict |
|---|---|
| **Prebuilt extension** | **Chosen for now.** Hermetic given the checksum, runs on any runner, no system cosign required. |
| Source-compiled from `github.com/sigstore/cosign/v3` via gazelle | **Deferred.** Goal: hermetic build-graph membership for the cosign binary. Blocker: the `pkg/providers/buildkite` package transitively pulls `github.com/buildkite/agent/v3`, which ships checked-in BUILD.bazel files referencing `@rules_go` from a context where it isn't visible. Patching cosign to drop the buildkite import leaves a phantom `@com_github_sigstore_cosign_v3//pkg/providers/buildkite` reference in the gazelle-generated `pkg/providers/all/BUILD.bazel` that doesn't yield to standard `gazelle_override` directives. Revisit when gazelle handles this case or when a deeper file-deletion patch is acceptable. |
| System cosign (`which cosign`) | Rejected. No hermeticity; CI runners would need cosign installed. |

## Predicate origin

Bazel-built JSON predicate (`slsa_predicate` rule), produced as a build-graph artifact from rule attributes + workspace status keys.

| Option | Verdict |
|---|---|
| **Bazel-built** | **Chosen.** Aligns with the project's "build system as policy enforcement point" instinct; the predicate is a hermetic build artifact whose contents are reproducible. |
| `actions/attest@v4` (GHA-built predicate) | Rejected. Predicate generated from runner context post-hoc, not from the build graph. Matrix-fan-out attest is one cold-start runner per image, inefficient. Pins the trust root to GitHub's OIDC issuer, forcing re-signing on any runner migration. |
| Bazel template + CI envsubst | Rejected. Introduces a CI-side mutation of the signed payload — small surface, but a surface; the OIDC cert is a better place for runtime fidelity than the predicate body. |

## Predicate schema

Phase 1: minimal `buildDefinition` + `runDetails.builder.id` (no `runDetails.metadata`, no `resolvedDependencies`). Phase 2 (deferred): full SLSA fidelity with materials sourced from the same `gather_metadata` aspect that drives `image_supply_chain`'s SBOM.

The minimal predicate avoids duplicating the SBOM (which is already attached as a separate `cyclonedx` attestation per image) and lets the OIDC certificate carry runtime fidelity (which workflow ran, when) rather than restating it in the predicate body.

## SLSA level

This design delivers **SLSA Build L2** as defined by SLSA v1.0:

| Level | Met? | Why / why not |
|---|---|---|
| L1 — provenance exists | ✓ | Every mirror image carries a SLSA v1.0 ProvenanceStatement attestation. |
| L2 — provenance is signed by a verifiable identity | ✓ | Keyless GHA OIDC; subject pinned to `ci.yaml@refs/heads/main`. |
| L3 — provenance generated by a hardened, isolated build platform | **No** | Our `slsa_predicate` rule runs as part of the Bazel build inside the user's repo. SLSA L3 explicitly disallows user-code-generated predicates; the platform must emit them. |

L3 was considered and **rejected for senku as currently positioned**:

| Path to L3 | Verdict |
|---|---|
| `actions/attest-build-provenance@v2` (GHA's certified-action chain) | Rejected. Re-couples the predicate path to GHA, undoing the runner-portability the `COSIGN_KEY` parameterization just achieved (see *Trust root* asymmetry). The action's L3 claim rests on a "trusted reusable-workflow" chain rather than platform isolation in the strict SLSA sense — softer than the spec implies. |
| Migrate the build to Cloud Build (or another natively-L3 platform) | Rejected for now. This is the architecturally honest path to L3 (platform-emitted provenance, à la Google's distroless), but it's a CI-stack rewrite. Justified by an enterprise consumer requirement, not by aspiration. |
| Stay at L2 | **Chosen.** No external consumer is asking for L3 today. Keep Bazel-native predicates with project-specific fields (`bazelTarget`, `monorepoVersion`) and runner portability; revisit if/when an L3-demanding consumer appears. Migration plan + trigger conditions tracked in [#192](https://github.com/arkeros/senku/issues/192). |

## Verification

External consumers run `cosign verify-attestation` on their own infrastructure (see `docs/oci-CONTEXT.md`). No internal verify gate substitutes for this — the registry's GAR copy uses IAM trust on a different perimeter. The CI publish job includes a negative test asserting the production verify policy rejects `gcr.io/distroless/static-debian12:nonroot` (signed-by-Google, wrong identity), so policy strictness is itself a tested invariant of the build.

| Option | Verdict |
|---|---|
| **CI negative test against signed-by-Google + consumer-facing docs** | **Chosen.** CI catches policy regressions (e.g. someone widens the verify regex); docs publish the verify command externally. |
| No negative test | Rejected. Silent regression risk: a future PR loosening the verify regex would never be caught until a consumer noticed. |
| Bazel `sh_test` faking Sigstore | Rejected. Mocking Fulcio certs is disproportionately complex relative to the regression net it provides. |
| Negative test against unsigned `nginx:latest` only | Weaker. Catches "no signature" but not "wrong identity"; signed-by-Google's-distroless is the stricter version that catches the real attacker-substitution case. |

## Operational considerations

**Where does a published signature live?**
Three places per image, all keyed on the same registry digest (`<repo>@sha256:<hex>`):

1. **OCI registry, as referrer artifacts.** GHCR stores the signature as `<repo>:sha256-<hex>.sig` and attestations as `<repo>:sha256-<hex>.att` (when `cosign` was invoked) or `<repo>:sha256-<hex>.intoto.jsonl` (when `actions/attest` family). Pull with `cosign tree <ref>` to enumerate.
2. **Sigstore Rekor (public transparency log).** Every keyless signature lands here. Search by digest at <https://search.sigstore.dev/> or via API: `curl -fsSL "https://rekor.sigstore.dev/api/v1/index/retrieve" -X POST -H 'Content-Type: application/json' -d '{"hash": "sha256:<hex>"}'`. The Rekor entry includes the full Fulcio cert (so you can audit the OIDC subject + issuer) and the `inclusionProof` for tamper-evidence.
3. **GitHub attestations API** (only when `actions/attest-build-provenance` is used — not in current L2 design). Not a concern today; relevant if the L3 plan in #192 ever lands.

**Debugging a failed `cosign verify-attestation`:**

| Symptom | Likely cause | First check |
|---|---|---|
| `no matching attestations: <error>` | Image was pushed but signing/attesting hasn't run yet (CI timing) | Re-run `bazel run :foo_sign` / `:foo_attest` against the digest |
| `certificate verification failed: x509` | Fulcio cert chain doesn't validate, usually a clock skew or stale TUF root | `cosign initialize` to refresh the trust root; check system time |
| `certificate identity for ... did not match any of the given identities` | The OIDC subject in the cert doesn't match the verify regex (drift between producer `BUILDER_ID` and consumer regex) | Inspect the cert: `cosign download attestation <ref> | cosign verify-blob-attestation --insecure-ignore-tlog ... --certificate-identity-regexp='.*'` to bypass identity check, then `jq` the cert's SAN |
| `transparency log entry not found` | Signature wasn't uploaded to Rekor (signer ran with `--tlog-upload=false`) | Check `cosign sign` / `cosign attest` invocation flags; we always upload by default |
| Verify succeeds locally, fails in CI | `cosign` version skew, or a runner without network access to Rekor | Pin `cosign` version in CI (we do, via the prebuilt extension); ensure runner egress to `rekor.sigstore.dev:443` |

**Where do signing operations show up in audit logs?**

- **Rekor**: every signature event is publicly auditable via the Rekor log index. Search by digest, by Fulcio cert subject, or by uploader IP.
- **GitHub Actions**: the workflow run that minted the OIDC token is visible at `https://github.com/arkeros/senku/actions/runs/<id>`. The cert's `OIDCBuildConfigUri` X.509 extension carries the run URL — use it to cross-reference back from a Rekor entry to the specific CI run.
- **GHCR**: registry pushes are logged in package activity (`https://github.com/users/arkeros/packages/...`). No native push-event audit log via API, but PR-merge events in `arkeros/senku` correlate to push events temporally.

**When something looks wrong (off-policy signature, unexpected publish event):** start at Rekor. Every off-policy signature attempt with `--tlog-upload=true` (the default) lands there. If a Rekor entry exists with a Fulcio cert whose subject doesn't match `CERTIFICATE_IDENTITY_REGEXP`, that's an attacker who minted a signature but consumers will (correctly) reject it — not an exposure, but a signal worth investigating.

**Rotation playbook (key rotation, workflow rename, runner migration):** there isn't a key to rotate (keyless OIDC). Workflow rename or runner migration changes the OIDC subject; consumers' verify policies break. Coordinate by:

1. Update `oci/cosign_policy.bzl` constants and propagate to producer + consumer surfaces (single-file change; everything else derives).
2. **Don't delete old signatures.** They remain valid against the old `BUILDER_ID` for any consumer who pinned to it. New images get new identity.
3. Announce the change in `oci-CONTEXT.md` and any external consumer documentation, with the date of cutover.
4. For runner migration specifically, see also #192 (L3 plan, which has its own runner-coupling implications).

## Addendum (2026-05-10): OCI 1.1 referrers

The "Where does a published signature live?" section above describes a *pre-3.x cosign* world where signatures land as `<repo>:sha256-<hex>.sig` and attestations as `<repo>:sha256-<hex>.att` sibling tags. **That is not actually how mirror_push has ever published.** Cosign 3.x defaults `--new-bundle-format=true`, and the bundle code path (`signDigestBundle` → `WriteBundle` for sign; equivalent `WriteBundle` for attest) writes via the **OCI 1.1 referrers API** (subject-bearing manifest, discoverable via `GET /v2/<repo>/referrers/<digest>`) regardless of `--registry-referrers-mode`. So mirror_push has been writing referrer manifests since the cosign-bzl rollout — there have never been any `.sig`/`.att` sibling tags on `ghcr.io/arkeros/senku/*`. (Empirically verified by direct probe: every `<digest>.sig` and `<digest>.att` lookup returns `MANIFEST_UNKNOWN`.)

No `referrers_mode` knob exists on the rules. Cosign 3.x dropped `--registry-referrers-mode` from `verify` and `attest` entirely (`attest` writes via referrers unconditionally on the bundle path); the flag survives only on `cosign sign` and there it's a no-op when `--new-bundle-format=true` (the default) — so plumbing it through `cosign_sign` would be dead weight, and on `cosign_attest` it would error at flag-parse time. An earlier iteration added the attr; it was reverted before any caller used it.

**What did change in this round (the work behind this addendum):**

- `oci/pkg/proxy` (the `distroless.io` registry proxy) gained handling for `GET /v2/<name>/referrers/<digest>`. Before, the path passed through to GHCR but the auth-scope was malformed because `findOp` didn't recognize `referrers`, so verifying via `distroless.io` didn't work. It does now.
- Verification guidance was published at `oci/distroless/README.md` with the cosign / oras / crane discovery commands.

**What does not change.** The trust root (keyless OIDC), the verify policy regex (`CERTIFICATE_IDENTITY_REGEXP` in `oci/cosign_policy.bzl`), the SLSA L2 claim, and the consumer-facing `cosign verify-attestation` command are all identical. Cosign 3.x discovers referrers by default and accepts no flag on `verify` to change that — consumers don't need a new flag either.

**No legacy-tag cleanup is required** — see the empirical note above. The `sha256-<hex>` tags visible via `crane ls` are spec-mandated OCI 1.1 referrers-fallback indices (an `application/vnd.oci.image.index.v1+json` listing referrer manifests), not legacy cosign siblings; deleting them would break referrers discovery for clients that fall back to the tag scheme.

## Next steps

Ordered by load-bearing-ness.

- **Source-compile cosign.** Replace the prebuilt extension with a gazelle-resolved `@com_github_sigstore_cosign_v3//cmd/cosign`. Blocker is logged in the *Cosign delivery* section above. Either land a deeper patch that physically removes `pkg/providers/buildkite/buildkite.go` from the cosign source (so gazelle has nothing to generate a phantom reference to), or wait for upstream to drop the buildkite OIDC provider, or for gazelle's resolver to honor `gazelle:exclude` for absolute self-references.
- **Phase 2 SLSA predicate.** Add `materials` (SLSA v1.0 `resolvedDependencies`) populated from the same `gather_metadata` aspect that drives SBOM generation. Makes the predicate self-contained: a verifier reading only the attestation gets a full materials manifest without chasing the SBOM sidecar.
- **Externalize `bazel/modules/cosign.bzl/`.** When (if) there's a second consumer, `git subtree split` the module into `github.com/arkeros/cosign.bzl` and switch senku from `local_path_override` to a versioned `bazel_dep`. Mechanical; the module's code is already module-boundary-clean.
- **Multi-issuer verify tolerance.** Currently the verify policy pins one OIDC issuer (GitHub Actions). When the day comes to migrate to BuildBuddy / Tekton / Cloud Build / Codeberg, the consumer-facing verify command needs to accept multiple issuers (or the project needs to dual-sign during the transition). Design when the migration is on the table, not before.
