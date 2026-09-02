# Platform provenance from slsa-github-generator

**Status:** Accepted, 2026-09-02. Supersedes [ADR 0006](./0006-bazel-native-cosign-mirror-signing.md).

Every image on the **Mirror** carries **Platform provenance** written by
`slsa-framework/slsa-github-generator`'s container generator, a reusable
workflow that runs in its own VM under its own OIDC identity after our build
has finished. The Bazel-written `slsa_predicate` is retired. Signature and
**SBOM** stay ours, bound by `mirror_push` in the build graph; provenance is the
one artifact the build graph cannot bind by definition, so CI attaches it, once
per digest, and a read-only job proves all three before any tag moves.

## Why now

The goal is the property Chainguard describes in ["Proven, not
promised"](https://www.chainguard.dev/unchained/proven-not-promised-chainguard-containers-achieves-slsa-build-level-3):
provenance a consumer can check that the build itself could not have forged.
ADR 0006 chose SLSA Build L2 and set "an L3-demanding consumer" as the trigger
for revisiting. No such consumer has appeared. What changed is the reading of
the trigger: a mirror whose whole pitch is verifiable evidence should not be
publishing the one kind of provenance its own build can lie in. This ADR
reverses 0006's level decision on that ground, and says so rather than
pretending the trigger fired.

Chainguard's claim rests on two legs: a control plane that writes provenance
outside the build worker, and a Coalfire assessment of that control plane. This
design supplies the first leg. Nothing supplies the second. The claim this
project makes is therefore **SLSA Build L3 provenance from a recognised L3
builder, self-assessed against the SLSA requirements, with no third-party
assessment**, and the README and `docs/images.md` say exactly that. A claim
without the evidence behind it is the **Silent zero** problem, one column over.

## Build provenance versus platform provenance

Both are signed statements bound to a **Digest** saying "this came from that".
They differ in who is allowed to be wrong.

*Build provenance* was the `slsa_predicate` output: the build graph wrote it,
so it could say what only the build knows — Bazel target, lockfile snapshot,
monorepo version. Its weakness is that the same graph that builds the image
writes the claim. A malicious rule, a poisoned cache entry or a compromised
runner makes the image and the claim lie together. SLSA calls that L2.

*Platform provenance* is written by something the build steps cannot touch.
Here that is GitHub's reusable-workflow machinery: the generator runs in a
separate VM with its own OIDC token after our job has finished, and records
what the platform observed — repository, commit, workflow, run ID, and the
digest we handed it. It cannot know the Bazel target because it never ran
Bazel. Its strength is that our build cannot forge it; its weakness is that it
attests "this digest came out of run N of workflow W at commit C" and nothing
about what happened inside run N.

They are not two formats of one fact. One says *what was built*, from the
inside; the other says *who built it and when*, from the outside. This design
ships the platform kind plus the signed **SBOM**, and lets the SBOM carry the
"what" — the same shape Chainguard ships.

## Decisions

### Provenance shape: platform only

| Option | Verdict |
|---|---|
| **Platform provenance only** | **Chosen.** One provenance statement per digest, ever. `slsa_predicate` and `mirror_push`'s `_attest` target are deleted. What the predicate carried — target, version — is the SBOM's and the tag's job. |
| Both: generator provenance plus the Bazel predicate under our own `buildType` | Rejected. Two things called provenance on one digest, two identities to explain, and the Bazel one is exactly the forgeable kind this ADR exists to stop publishing. |
| Bazel predicate only (stay at L2) | Rejected. See *Why now*. |

### Trigger: every push to `main`

Publishing stays on push to `main`, as it is. The draft of this ADR proposed
`v*` release tags; the repository has none, its weekly `YYYY.WW` tags are
calendar checkpoints, and 0006 already rejected tag-bound identity for that
reason. Tag-triggered releases would also have reintroduced "a CVE fix does not
ship until someone tags", which trunk-based publishing does not have. The
generator does not care which event calls it.

### Identity: two signers, one policy file

The signature and the SBOM attestation are signed by `ci.yaml@refs/heads/main`
as before. Provenance is signed by the generator's own workflow identity,
`slsa-framework/slsa-github-generator/.github/workflows/generator_container_slsa3.yml@refs/tags/vX.Y.Z`.
That certificate says nothing about *our* repository — every project on GitHub
using the generator produces provenance with the same signer — so which source
built the image lives inside the predicate, and verification has to read it.
`slsa-verifier verify-image --source-uri github.com/arkeros/distroless
--builder-id <generator workflow>` does that. `cosign verify-attestation` with
the generator's identity regexp alone would accept another project's
provenance on our digest.

| Option | Verdict |
|---|---|
| **`slsa-verifier` for provenance, `cosign` for signature and SBOM** | **Chosen.** One tool per artifact, and it is what the generator project documents. |
| `cosign verify-attestation` plus a CUE policy on `invocation.configSource.uri` | Rejected. A second policy language to maintain for what slsa-verifier does with two flags. |

`oci/cosign_policy.bzl` stays the single source of truth and gains the
provenance builder ID and source URI beside the existing signer regexp and
issuer. CI, the docs and the negative tests all derive from it.

### One provenance per digest

The generator mints a fresh statement per call, so calling it for a digest that
already has one violates the invariant. The publish job decides which digests
are pending by *verifying* each pushed digest with slsa-verifier and
classifying the result: success means attested; `no matching attestations`
means pending; any other failure fails the job, because a Rekor or TUF outage
must not be read as "unattested" and answered with a second statement.

| Option | Verdict |
|---|---|
| **Verify each digest at the registry** | **Chosen.** The registry is content-addressed and is the source of truth. Self-healing: a generator run that dies leaves a digest without provenance, and the next push to `main` attests it. |
| Referrer existence only (`cosign tree`, `oras discover`) | Rejected. Any garbage referrer would suppress attestation. |
| A checked-in or bucket-stored list of attested digests | Rejected. Does not notice a failed generator run; drifts from the registry. |
| Compare against the previous tag | Rejected. Resolving `:latest` from the registry is a race the CI comments already warn about. |

Digests are stable across commits when content is unchanged — `created` comes
from the lockfile and index annotations carry no commit — so the invariant
means something: a push that touches only `web/` re-attests nothing.

### Tags move after verification

`mirror_push`'s `_push` now pushes by digest only. A new `_tag` target applies
the tag list with `crane tag` against the digest the build produced, and the
`release` job runs `_tag` for each published target only after `verify` has
proven all three artifacts on every published digest. Tags are therefore all-or-nothing per run:
a release is the run, and a partial release is not a release. Before this, a
tag could point at a digest for the minutes between push and attestation, and a
consumer verifying at admission in that window was refused.

| Option | Verdict |
|---|---|
| **`_tag` via crane, run by `release`** | **Chosen.** Crane is already a Bazel target from the go.mod dependency; the tag templates and the digest both already live in the build graph, so the tag list stays declared beside the image. |
| Push tags with the image and accept the window | Rejected. It was the design, and it is a consumer-visible defect. |
| A second `image_push` with the tag list | Not needed; equivalent, but crane makes the operation explicit. |

### Jobs: publish, provenance, verify, release

| Job | Holds | Does |
|---|---|---|
| `publish` | `packages: write`, `id-token: write`, `environment: prod` | Push by digest, sign, attest SBOM; emit the pending `{image, digest}` list and the published `{target, ref}` list |
| `provenance` | `actions: read`, `id-token: write`, `packages: write` | Matrix over pending digests, one generator call each |
| `verify` | `packages: read` | Refuse an empty list; `cosign verify`, `cosign verify-attestation --type=cyclonedx`, `slsa-verifier verify-image` per published digest; both negative tests |
| `release` | `packages: write`, `environment: prod` | Run `_tag` for exactly the published targets |

`verify` is deliberately read-only: neither cosign nor slsa-verifier needs a
token to verify public artifacts, so the job that holds the policy cannot
mutate the registry. `release` tags from the same `{target, ref}` list `verify`
checked rather than from a fresh query, so the tagged set is the verified set
by construction, and an empty list fails `verify` rather than passing
vacuously.

The Cloud Run deploys gate on `test` alone, through `push-gar`, and not on any
mirror job. Before this ADR they waited on `publish`, on the argument that
production should never run a digest the mirror refused to name. That coupling
was dropped deliberately: the deployed copies travel the **Deploy path** and
are trusted by IAM rather than by signature, and a mirror failure unrelated to
the two services — a Rekor outage, a base-image verify failure, a deleted
negative-test fixture — would otherwise block a Registry or Directory fix from
shipping. The cost is that production can run a digest whose mirror copy
failed verification; `test`, which every push to `main` reruns, is the gate
that stands in for it.

The cost 0006 counted against `actions/attest` applies here too: one fresh
runner per digest. A Debian lockfile bump changes most of the 20 mirror images
at once, so that is up to 20 short jobs. Accepted; lockfile bumps are weekly
and the alternative is not having L3.

### Scope: every `mirror_push` target

Twenty targets: the sixteen under `//images` and the Registry, the Directory
and two devtools images. Anything less would make "everything on the Mirror is
signed and attested" a two-tier statement.

### Workflows: `pr.yaml` and `ci.yaml`

`pr.yaml` runs on `pull_request` and does one thing, `bazel test //...`, with a
read-only cache identity and no environment. `ci.yaml` runs only on push to
`main`, every job under `environment: prod`. Every `if:` on event type and the
fork-skip branch in the auth step disappear. The `Test` job name the
`main-checks` ruleset requires is unchanged. The cost is thirty duplicated
lines and `main` test runs appearing in the deployments log.

### Cache writes are `main`-only

Cache poisoning is the textbook way to defeat platform provenance: a branch
writes a bad action result, `main` reads it, the generator signs the outcome.
Until now any branch in the repository could write the cache. The cache-write
account is rebound from `attribute.repository` to `attribute.environment/prod`,
whose deployment branch policy names `main` and nothing else; a new viewer-only
account bound to `attribute.repository` serves pull requests, and
`--config=gcs-readonly` never attempts an upload. Ref binding and environment
binding are equally strong against this threat — GitHub validates both before
minting the token, and both pass a `pull_request_target` job, which is why this
repository uses none — but the environment is the existing pattern, is
Terraform-managed, and shows every write-capable run in the deployments log.

`main` reads only what `main` wrote. The deploy account's existing cache write
binding stays: it is only reachable behind the same environment.

### Generator pinning

The generator refuses SHA references and requires a full `vX.Y.Z` tag. That is
weaker than the digest pinning everything else here uses and cannot be
strengthened from our side. Renovate bumps it as a minor or patch update;
the generator is excluded from auto-merge so a human sees every bump to the
trust root of the L3 claim.

## Threat model

Carried over from 0006 where still true; changed rows marked.

| Capability | Defended? | Notes |
|---|---|---|
| Compromised registry or MITM substitutes an image at our digest | **Yes** | Signature, SBOM and provenance all bind publisher identity to the digest. |
| Compromised CI workflow file on a branch | **Yes** | Signer regexp is workflow-bound to `ci.yaml@refs/heads/main`; PR events cannot mint OIDC tokens with `id-token: write` in `pr.yaml`. |
| **Compromised build step forges provenance** | **Yes (changed)** | The generator writes provenance in a separate VM under its own identity. The build job can push any digest it likes, but cannot make the platform say it came from a different commit or workflow. |
| **Compromised build step forges signature or SBOM** | **No (unchanged)** | Both are signed by our job. A runner compromise mints them with our identity for the duration of the run. Detection via Rekor. |
| **Cache poisoning from a branch** | **Yes (changed)** | Writes are `main`-only via the `prod` environment. |
| Another project's generator provenance presented on our digest | **Yes** | `--source-uri` is checked inside the predicate; CI's negative test asserts `ghcr.io/kyverno/kyverno`, which carries real generator provenance, is rejected for source mismatch. |
| Compromised generator project or its release tag | **No** | The generator is the trust root. Renovate bumps are not auto-merged; the tag is the strongest pin the generator permits. |
| Compromised GitHub runner isolation or OIDC | **No** | The whole L3 claim rests on GitHub separating reusable-workflow jobs. No independent assessment. |
| Compromised Sigstore root | **No** | As 0006. |
| Raw `image_push` to the mirror prefix | **Yes (analysis-time)** | `mirror_push_enforcement_aspect`, unchanged. |
| Tag moved to an unattested digest | **Yes (changed)** | `_tag` runs only in `release`, after `verify`. |

## Consequences

- One provenance per digest, from the platform. The Bazel predicate, its golden
  test, `BUILDER_ID` and `BUILD_TYPE_URI` are gone; 0006's phase-2 plan
  (materials from the SBOM aspect) dies with them.
- Consumers pin two identities: ours for signature and SBOM, the generator's
  plus our source URI for provenance. `docs/images.md` prints both.
- The provenance predicate type is SLSA v0.2, which is what the generator
  emits and what `cosign --type=slsaprovenance` names. The Directory's
  constant said `v1`; corrected.
- The **Directory** keeps rendering only the SBOM. Rendering platform
  provenance — commit and workflow run, verified against the generator identity
  and our source URI in Go — is a follow-up.
- The **Deploy path** stays IAM-trusted with no provenance check, and the knife
  CLI release stays as it is; neither is a container on the Mirror.
- `verify` runs on every push to `main` against the public Sigstore
  infrastructure; a TUF or Rekor outage blocks releases rather than being read
  as a policy failure. That is the intended direction of the error.
- The generator's own limitations are ours: `pull_request` is unsupported (not
  used), the image must already be in the registry (it is), and the reusable
  workflow needs `packages: write` even though it only writes referrers.

## What stays from ADR 0006

The trust root (keyless OIDC, Fulcio, Rekor), the `mirror_push` policy unit
and its enforcement aspect, cosign delivered as a prebuilt through the multitool
lockfile, the `cosign.bzl` module location, and the negative test against
Google's distroless image are all unchanged and are not restated. The
operational content — where artifacts live, debugging failed verifies, audit
trails, the rotation playbook — moved to
[`docs/mirror-signing-runbook.md`](../mirror-signing-runbook.md).
