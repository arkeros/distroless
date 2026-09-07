# Cloud Build + GAR + Hetzner distribution

**Status:** Deferred, 2026-09-07. Not started, and should not be started until
a trigger below fires. Written down because the analysis behind it was
expensive and the numbers rot.

Three changes that only make sense together: move the build from GitHub
Actions to Cloud Build, move the **Mirror** from GHCR to Artifact Registry, and
turn [`//oci/proxy`](../oci/proxy/proxy.go) into a blob-caching registry served
from Hetzner bare metal. Any one of them alone is worse than what we do today.

## Why they are coupled

The chain is forced, in this direction only:

1. Cloud Build is a recognised SLSA Build L3 builder — it *"performs builds in
   isolated and ephemeral environments"* and its provenance is
   *"service-generated, verifiable, and non-forgeable."* Adopting it would
   replace `slsa-github-generator` as the trust root of
   [ADR 0014](../docs/adr/0014-platform-provenance-slsa-github-generator.md).
2. But **Cloud Build only generates build provenance for artifacts stored in
   Artifact Registry.** Our public surface is `ghcr.io/arkeros/distroless/*`.
   So adopting Cloud Build means moving the Mirror to GAR, or getting no
   provenance for the images that need it.
3. But GAR bills internet egress per pull, where GHCR is free. A public mirror
   on GAR pays more the more it is used — cost scales with adoption, not with
   our build volume. So the Mirror can only live on GAR behind a caching proxy
   that absorbs public reads.
4. Hetzner dedicated servers have traffic *"unlimited and free of charge"* on
   the standard 1 Gbit uplink, so that is where the bytes get served. The proxy
   must then **store** blobs rather than redirect to them.

Break the chain anywhere and the remaining pieces are strictly worse than
today. That is the main thing this document exists to record.

## Why not now

Today's shape is already the cheap one, for a reason that is easy to miss:
`//oci/proxy` forwards GHCR's `307` to a presigned URL (`proxy.go:155–163`), so
**it never carries blob bytes.** GHCR's CDN serves them, globally, for free.
The proxy handles manifests and redirects — kilobytes — which is why it runs
happily as a stateless multi-region Cloud Run service.

Caching bytes locally would replace a free global CDN with one box in one
datacentre. It saves nothing while GHCR is free, and costs latency outside
Europe, a single point of failure on the public distribution path, and a disk
to operate.

## Triggers

Start this only if one of these is true:

- **GHCR begins billing bandwidth or storage for public packages.** It is
  *"currently free"*, with GitHub promising *"at least one month in advance"*
  of any change. That month is the implementation window, which is why this
  document exists rather than the code.
- An L3-demanding consumer rejects `slsa-github-generator` provenance but
  accepts Cloud Build's. (ADR 0006 set "an L3-demanding consumer" as the
  trigger for the last provenance change; same bar.)
- GitHub-hosted runner minutes stop being free for this repository.

Cost alone is not a trigger. The GCS cache that started this analysis is gone
(see below); CI egress is already $0.

## What it would cost

Measured from the `senku-prod-bazel-cache` bucket over 2026-08-31 → 09-07,
before it was deleted. Kept because it is the only real data we have on this
repository's byte and request volume:

| | measured (8 days) | /month |
|---|---|---|
| Egress to GitHub-hosted runners | 1,685 GB | ~6,300 GB |
| Class B ops (read + stat) | 27.6M | ~103M |
| Class A ops (write) | 576k | ~2.2M |
| Bazel invocations | 664 | ~2,490 |
| Actions per invocation | ~22k | |
| Build wall-clock | 2,258 min | ~8,500 min |
| Machine-minutes (est.) | ~4,600 | ~17,300 |

Public *pull* volume is unmeasured — we have never served it ourselves, GHCR
has. **Measure it before committing to any design that meters it.**

Unverified figures, flagged deliberately: Artifact Registry storage and egress
rates, and Cloud Build per-build-minute rates. Google's pricing pages truncate
on fetch and neither number could be quoted. ~17,300 build-minutes/month
clears any Cloud Build free tier by a wide margin; the rest needs checking
against the console before anyone plans on it.

## What it breaks

Moving off GitHub Actions invalidates most of ADR 0014's chain:

- The signer regexp bound to `ci.yaml@refs/heads/main`
- The `verify` job and its `--source-uri` check inside the predicate
- The negative test asserting `ghcr.io/kyverno/kyverno` is rejected
- Row 224 of the threat model (workflow-bound signer identity)
- The `prod` environment as an identity gate — GitHub validates the deployment
  branch policy before minting OIDC tokens, and Cloud Build has no equivalent
  tied to a GitHub ref

This is an ADR-0014-superseding change, not an infrastructure tweak. Budget for
rewriting the provenance chain, not for editing YAML.

## Trust analysis

The distribution proxy is **downstream** of signing, and that is what makes
Hetzner acceptable here when it is not acceptable for a build cache.

A build cache sits upstream: poisoning it produces images that are
*legitimately signed* and wrong, which is exactly the threat ADR 0014 row 227
defends against. A distribution cache sits downstream: verify digests on
ingest and serve by digest, and a compromised box cannot forge content —
substitution fails digest comparison and `cosign verify`.

Two residual risks:

- **Rollback.** A compromised proxy can point a mutable tag at an older, still
  validly-signed digest. Verifying clients catch it via provenance; a plain
  `docker pull` by tag does not. [ADR 0017](../docs/adr/0017-tag-ledger.md)'s
  ledger is the existing lever here.
- **Availability.** One box replaces a CDN. A public mirror going down is a
  worse failure than a slow one.

Hetzner Cloud and dedicated servers expose **no TPM or vTPM**, so there is no
hardware-rooted attestation of what the box is running. That is tolerable
downstream of signing and disqualifying upstream of it — the reason this plan
puts only distribution there, and never the build.

## Sequence

Each step is independently revertible and leaves a working system:

1. **Measure public pull volume.** Add byte counting to the Cloud Run proxy and
   run it for a month. Without this the GAR egress estimate is a guess.
2. **Blob storage in `//oci/proxy`**, still upstream-of-GHCR, still on Cloud
   Run or a single box. Content-addressed store, **digest-verified on ingest**
   — the property the whole trust argument rests on, so it gets a failing test
   first. LRU eviction against a disk quota. Range requests via
   `http.ServeContent`. `singleflight` on cold blobs or N clients become N
   upstream fetches. The delicate `blobs` cache-control case at
   `proxy.go:155–163` *disappears*: blobs become
   `public, max-age=31536000, immutable` like digest-addressed manifests.
3. **Move serving to Hetzner bare metal**, still upstream-of-GHCR. Validates
   the box, the traffic assumption and the ops burden while the free CDN is
   still there to fall back to.
4. **Cloud Build pivot**, and only then repoint upstream to GAR. This is the
   irreversible step and the one that supersedes ADR 0014.

Stopping after 2 or 3 is a legitimate outcome: it is the GHCR-billing hedge on
its own, without touching provenance.

## Alternatives considered

All evaluated 2026-09-07 against the measured volume above.

| Option | Verdict |
|---|---|
| **Keep GHCR + GitHub-hosted runners + Actions cache** | **Chosen.** $0, no external trust boundary, no ops. Slower builds. |
| Cloudflare R2 + `bazel-remote` | Cheapest cache by far — free egress, ~$51/mo on measured request volume. Rejected: puts the cache outside the GitHub trust boundary, and the `main`-only write gate degrades from short-lived WIF tokens to a stored API secret. |
| Namespace Bazel cache | Bills **$0.10 per 1,000 Action Cache hits**. At ~22k actions × ~2,490 invocations/month that is ~55M hits — $1,100–5,500/mo depending on how much a warm cache volume absorbs. Wrong pricing model for this build's action density. |
| Namespace runners + cache volumes | Viable: no egress, no per-hit meter, no fleet. But cache volumes are not a REAPI endpoint, so `rules_img` lazy push cannot use them. |
| BuildBuddy Cloud | Free tier is 100 GB cache transfer/month; we need 3,300–6,300 GB. 33–63× over. |
| Self-hosted runners on GCE | Free egress with a *regional* bucket (multi-region → GCE is **not** free). Blocked by GitHub's warning that *"forks of your public repository can potentially run dangerous code on your self-hosted runner machine"*, plus a fleet to operate. |
| Self-hosted cache on Hetzner | Rejected on trust: upstream of signing, tenant-operated, no attestation primitive. Flips ADR 0014 row 227 back to undefended. |
| GCS bucket (what we had) | ~$815/mo, 93% of it internet egress to GitHub-hosted runners on Azure. Deleted 2026-09-07. |

## Notes

- The GCS bucket, its two service accounts, `infra/cache.tf`, the
  `setup-bazel-remote` action and every `--config=gcs*`/`--config=bazel-remote*`
  wiring were removed on 2026-09-07. Recover the proxy action from git history
  rather than rewriting it if a remote cache ever returns.
- `rules_img` lazy push was removed with the cache — it needs a gRPC REAPI
  endpoint. Any future cache that wants it back must speak REAPI, not S3.
- Whatever cache returns, the property to insist on is that the **write path is
  gated by an identity system rather than a shared secret**. That is what made
  the GCS design sound, independent of what it cost.
