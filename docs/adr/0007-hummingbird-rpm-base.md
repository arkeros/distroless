# Hummingbird-based distroless

Senku's distroless surface (`distroless.io/*`) is Hummingbird-derived. All images consume RPMs from `koji-s3-cache.hummingbird-project.io` as their package base, ship `ID=hummingbird` in `/etc/os-release` for correct vulnerability-scanner routing, and brand as "distroless.io (Hummingbird-derived)" in `PRETTY_NAME`. The competitive thesis is real consumer-scanner zero on glibc-bearing images — matching Chainguard's `glibc-dynamic` and Red Hat Hardened Images on an OSS-pure supply chain — plus uniform supply-chain identity across the senku surface (no mixed Debian/Hummingbird footguns in consumer SBOMs).

## Why not Debian sid

[[oci/distroless/common/variables.bzl]] currently silences three glibc CVEs (CVE-2026-5435, 5450, 5928) via `DEBIAN_WONTFIX_CVES`, all unfixed in Debian sid's `libc6 2.42-15`. Tracking sid was chosen specifically to dodge Debian-stable's "no-DSA Minor" backport-skipping policy, but sid still inherits whatever upstream glibc ships, and the three CVEs sit unpatched upstream. The Hummingbird Project ships `glibc-2.42-13.hum1` containing backports for all three (advisory `RHSA-2026:12740` per Red Hat's CVE API). For glibc-bearing images this is the only OSS-pure source of those backports.

## Why not the alternatives

| Distro family | Verdict | Why |
|---|---|---|
| **SUSE SLE 16 / openSUSE Leap 16** | Rejected | OVAL feed acknowledges the three CVEs as `affected, no fix shipped` (verified empirically by fetching `suse.linux.enterprise.server.16.0-affected.xml.bz2`). The green `bci-micro:16.0` grype/trivy scan is feed-ingestion lag, not security posture. SUSE shares Debian's wontfix list under a different label. |
| **RHEL standard (UBI / `registry.access.redhat.com/ubi9-minimal`)** | Rejected | All three CVEs in `Fix deferred` / `Affected` status on RHEL 6-10 per Red Hat's security data API. Empirically reproduces: scanning `ubi9-minimal` shows CVE-2026-5435 alongside other open glibc CVEs. RHEL standard is *worse* than what we ship today. |
| **AlmaLinux 9** | Rejected | Inherits RHEL's "Fix deferred" status; same upstream-glibc reality. Tested empirically (`ID=almalinux` os-release on the same payload → 3 wontfix Mediums). |
| **Fedora rolling** | Rejected | Aggressive CVE response but 13-month lifecycle is hostile to a stable consumer surface; would mean major-version migrations every year. |
| **Wolfi (via Chainguard)** | Rejected | Single-vendor (Chainguard), the entity we're competing against. Adopting their distro forecloses the differentiation. |
| **stagex** | Rejected per [[docs/research/deb-vs-apk-vs-oci-components.md]] | Catalog ceiling (~250 packages) too tight for a base-distro role. |
| **Hummingbird** | **Chosen** | OSS-permissive EULA (GPLv2 redistribution); ~17,400 packages in the public repo; multi-arch (amd64, arm64, ppc64le, s390x); source RPMs published; GPG-signed; ships actual backports for the CVEs Debian/SUSE/RHEL all defer. Per Red Hat's CVE API: `glibc-2.42-13.hum1` is the only acknowledged-fixed version anywhere in the public field. |

## Scope: all senku images

| Image | Composition |
|---|---|
| `static` | zero-by-exclusion (no glibc) on Hummingbird base — 3 upstream packages: `tzdata`, `ca-certificates`, `mailcap` |
| `cc`, `bash`, `nginx`, `python`, `nodejs` (future) | glibc-bearing on Hummingbird base — adds `glibc`, `glibc-common`, `libgcc`, etc. |

All images ship a single uniform identity (`ID=hummingbird`) and use the same supply chain. The earlier draft of this ADR carved `static` out on the grounds that "no glibc → no security benefit"; that reasoning was rejected because brand/identity coherence and rules_rpm amortization both argue for uniform sourcing. A senku surface split between Debian-`static` and Hummingbird-`cc` would leak the migration internals into the public contract and double the rebuild-cadence infrastructure for no security gain.

### Static composition on Hummingbird

| Layer | Source | Notes |
|---|---|---|
| `tzdata` 2026a-1.1.hum1 | Hummingbird, noarch | Timezone data |
| `ca-certificates` 2025.2.80_v9.0.304-7.1.hum1 | Hummingbird, noarch | CA bundle |
| `mailcap` 2.1.54-10.1.hum1 | Hummingbird, noarch | `/etc/mime.types` |
| rootfs / passwd / home / group / tmp / os-release | senku-synthesized | Existing tar generators in [[oci/distroless/common/BUILD]]; os-release rewritten with `ID=hummingbird` per the attribution block above |
| EULA | senku-synthesized (cp from Hummingbird's `/usr/share/hummingbird-release/EULA` at lock time) | Placed at `/usr/share/licenses/hummingbird/EULA` |

This is *fewer* upstream packages than Debian-static currently consumes (which pulls `base-files`, `netbase`, `tzdata`, `media-types`). The Hummingbird `setup`, `filesystem`, and `hummingbird-release` packages are deliberately *not* consumed — senku synthesizes the rootfs skeleton, user database, and os-release directly, which gives tighter control of identity and attribution than the upstream rpms would.

Image-by-image migration uses the existing `*_DISTROS` matrix axis — see [[oci/distroless/matrix.bzl]]. Static migrates first because it has the smallest dep closure (no glibc transitive graph) and exercises the rules_rpm path on the simplest case.

## Identity claim: `ID=hummingbird` in `/etc/os-release`

For consumer vulnerability scanners (grype, trivy) to route glibc lookups to the `hummingbird-1` secdb provider — which acknowledges the backported fixes — our images must ship exact `ID=hummingbird` in `/etc/os-release`. Empirically verified by mutation tests on `registry.access.redhat.com/hi/curl:latest`:

| `/etc/os-release` ID | grype distro routing | CVE-2026-5435/5450/5928 |
|---|---|---|
| `ID=hummingbird` | hummingbird 20251124 | **0 matches (real zero)** |
| `ID=rhel` | rhel 10.0 | 5 reported incl. CVE-2026-5435 |
| `ID=debian` | debian-13 | 3 reported as Critical/High wontfix |
| `ID=distroless ID_LIKE="hummingbird"` (remix pattern) | (no provider matched) | silent zero — same trap as `bci-micro:16.0` |
| `ID=almalinux ID_LIKE="rhel centos fedora"` | almalinux 9.5 | 3 reported as Medium |

Grype does **not** use `ID_LIKE` as a fallback router (confirmed empirically). The only os-release shape that produces real-zero routing is the exact identifier `hummingbird`. The AlmaLinux-style remix pattern produces silent-zero — the fraud-by-silence anti-pattern we explicitly disqualified for SUSE in this same investigation.

### Attribution

The os-release makes derivation explicit:

```
ID="hummingbird"
ID_LIKE="rhel fedora"
NAME="distroless.io"
PRETTY_NAME="distroless.io (Hummingbird-derived)"
VERSION_ID="<hummingbird snapshot revision>"
HOME_URL="https://distroless.io/"
SUPPORT_URL="https://github.com/arkeros/senku/blob/main/oci/distroless/README.md"
```

The `ID` field is functionally a scanner-routing key. `NAME` and `PRETTY_NAME` carry the actual branding. Consumers see "distroless.io (Hummingbird-derived)" everywhere a human reads the image; scanner tooling reads `ID` to route correctly.

## Consumption mechanism

| Option | Verdict |
|---|---|
| **`rules_rpm` (Bazel-native, rules_jvm_external-style) + two Go binaries** | **Chosen.** New Bazel ruleset: `hummingbird.install(...)` module extension takes the package list inline in `MODULE.bazel`, pins via a checked-in `hummingbird_install.json` (JSON, not YAML), `bazel run @hummingbird//:pin` regenerates the lockfile from the declared list. Per-package tar emission shape-compatible with the existing `@debian//pkg/arch` label convention, so [[oci/distroless/matrix.bzl]] and image BUILDs need only the `_DISTROS` axis bumped. |
| YAML manifest + JSON lockfile (rules_distroless apt mimic) | Rejected. Two formats for no senku gain; rules_distroless inherited the split from pip/apt conventions that don't carry over to bazel-native ecosystems. |
| Starlark-only lockfile (`hummingbird.lock.bzl`) | Rejected. JSON is data, not code — easier to query with `jq`, dependabot-friendly, clean GitHub diffs, no `load()` semantics. Bazel's `json.decode()` makes JSON parsing trivial in the extension. |
| OCI-component overlay (per [[docs/research/deb-vs-apk-vs-oci-components.md]] §7) | Rejected for Hummingbird specifically. Hummingbird publishes ~10 application images (`hi/curl`, `hi/nodejs`, ...) but no standalone building-block images (no `hi/glibc`, no `hi/static`, no `hi/cc`). Their building-block-bearing artifact is the RPM repo, not a container image. Extraction from app-image layers is fragile and version-coupled. |
| `rules_distroless` patched to accept rpm repos | Rejected. Upstream is apt-shaped; bending it to rpm semantics is a larger refactor than a fresh module, and would muddy `@debian//` vs `@hummingbird//` boundaries. |

### Module extension shape

```
hummingbird = use_extension("//bazel/modules/rules_rpm:extensions.bzl", "hummingbird")
hummingbird.install(
    name = "hummingbird",
    repo_url = "https://koji-s3-cache.hummingbird-project.io/packages.redhat.com/api/pulp-content/public-hummingbird",
    gpg_key = "//bazel/modules/rules_rpm:hummingbird-release.pgp",
    architectures = ["x86_64", "aarch64"],
    packages = [
        "tzdata", "ca-certificates", "mailcap",
        "glibc", "glibc-common", # ...
    ],
    lock_file = "//:hummingbird_install.json",
)
use_repo(hummingbird, "hummingbird")
```

`hummingbird_install.json` is pin-tool output: hierarchical `packages[name][arch]` keyed structure carrying version, sha256, and rpm path per (package, arch) pair. `noarch` packages have one nested key; arch-specific packages have `x86_64`/`aarch64`. Never hand-edited.

## Closed manifest, not solver

Package names listed inline in `MODULE.bazel` are the canonical intent. The pin tool (`bazel run @hummingbird//:pin`) resolves them against `primary.xml.gz` and errors if any listed package is missing — closed-manifest semantics, no auto-transitive-expansion. Manual maintenance cost is bounded because senku's package universe is small (~50–100 packages across all images); solver-grade flexibility would cost more than it saves. Same posture as the existing [[oci/distroless/debian.yaml]] approach, just expressed in Starlark instead of YAML.

## Snapshot strategy

`repomd.xml` carries a `revision` (Unix timestamp; observed values: `1778835791` early-afternoon 2026-05-15, `1778852516` ~3 hours later — Hummingbird's repo updates multiple times per day). The lockfile pins both `revision` and per-rpm SHA256. `revision` gives cache-friendly URLs and a moment-in-time anchor; per-rpm digests are the build-graph determinism backstop independent of URL stability. Closest analog to `snapshot.debian.org`'s timestamp pinning that exists in rpm-land — no public time-machine service for rpm distros, but the lockfile *is* the snapshot.

**Implementation note (CDN behavior):** Hummingbird's CDN rejects `HEAD` requests with HTTP 403 and returns `302` redirects on `GET` to S3-backed URLs. The pin tool must use `GET` with redirect-following (Go's `http.Get` default; `curl -fsSL` equivalent). Never use `HEAD` to probe — even existence checks need a `GET` of metadata files, then inspect the body.

## rpmdb sqlite — two-binary architecture

Hummingbird images use sqlite-based rpmdb at `/usr/lib/sysimage/rpm/rpmdb.sqlite` (verified empirically on `hi/curl`). syft's rpm-db cataloger reads this exact path; without it, syft falls back to CPE-based detection against syft-generated CPEs whose vendor strings don't match NVD's canonical `cpe:2.3:a:gnu:glibc:*`, producing CPE-mismatch silent zeros (the same trap we identified on `bci-micro:16.0`).

**Why apk's single-tool pattern doesn't work for rpm.** [[devtools/build/tools/wolfi-apk-extract/main.go]] gets away with single-package emission because `/lib/apk/db/installed` is flat text — per-package fragments naturally concatenate when `flatten(deduplicate=True)` stacks the layers. rpmdb is binary sqlite; identical-path collisions clobber rather than merge. A single-binary rpm-extract emitting per-package sqlite at `/usr/lib/sysimage/rpm/rpmdb.sqlite` would cause the last tar listed to win — syft would see one package and consumers would see a one-package image regardless of what's actually in the layer. This is the fraud-by-silence anti-pattern the empirical investigation behind this ADR explicitly disqualified.

**Solution: two Go binaries, fan-in at the layer level.**

| Binary | Inputs | Outputs |
|---|---|---|
| `hummingbird-rpm-extract` (per-package) | One `.rpm` | `content.tar` (allow-listed paths, canonical uid/gid/mtime, **no rpmdb writes**) + `header.blob` (raw RPM header binary, ~10–50 KB) |
| `rpmdb-merge` (per-image) | N `header.blob` files | `rpmdb.tar` containing `/usr/lib/sysimage/rpm/rpmdb.sqlite` with one Packages row per input blob plus secondary indexes (Name, Basenames, Group, Requirename, Providename) |

In the BUILD seam:

```
flatten(
    name = "hummingbird_static_amd64_layer",
    tars = [
        "@hummingbird//tzdata/amd64:content",
        "@hummingbird//ca-certificates/amd64:content",
        "@hummingbird//mailcap/amd64:content",
        ":rpmdb",                              # merged from the same three
        "//oci/distroless/common:rootfs",
        # ... senku-synthesized rootfs/passwd/etc ...
    ],
)

rpmdb_merge(
    name = "rpmdb",
    headers = [
        "@hummingbird//tzdata/amd64:header",
        "@hummingbird//ca-certificates/amd64:header",
        "@hummingbird//mailcap/amd64:header",
    ],
)
```

Per-package extraction is independent and cacheable; the merge runs once per image and reruns only when the package set changes. Pure-Go via `modernc.org/sqlite` keeps the toolchain hermetic.

### Determinism

`modernc.org/sqlite` keeps the *library* hermetic; the *output bytes* take explicit work. `rpmdb-merge` must:

- **Sort inputs before insertion.** rpmdb's `Packages` table is `INTEGER PRIMARY KEY` (rowid alias); auto-assigned rowids follow insertion order, and b-tree page layout derives from rowids. Sort `header.blob`s by `(Name, EVR, Arch)` before the insert loop, and apply the same canonical ordering to every secondary-index population pass. Never iterate a Go map directly into a write.
- **Zero senku-controlled install metadata.** The sqlite is synthesized directly (no `rpm --install` call), so every "install-time" tag is our choice, not librpm's. Set `RPMTAG_INSTALLTIME=0`, `RPMTAG_INSTALLTID=0` (librpm's default of `time(NULL)` is the canonical determinism trap), and fix `RPMTAG_INSTALLCOLOR` / `RPMTAG_INSTPREFIXES`. Pre-verify on `hi/curl` that syft's rpm-db cataloger reads only Name/EVR/Arch from each header — if any install-time field turns out to be consumed, switch that field to a canonical non-zero constant rather than zero.
- **Pin SQLite knobs explicitly.** `PRAGMA page_size = 4096` before any write; `PRAGMA journal_mode = OFF` during build with a clean close so no `-wal`/`-shm` artifacts leak into the tar; no `AUTOINCREMENT` on any table (keeps `sqlite_sequence` out of the file entirely); single transaction, then `VACUUM` to normalize free-page layout.
- **Schema fixture, version-pinned.** Check in the schema dump from `hi/curl`'s actual `rpmdb.sqlite` as a test fixture; `rpmdb-merge` builds *that* exact schema. If a future Hummingbird rpm-version bump shifts the schema, the fixture mismatch fails loudly instead of drifting silently into a "Packages table + ~10 secondary indexes" handwave. Schema reference for the implementer: rpm-tools' `rpmdb.c` and `librpmstrpool`.
- **Byte-compare determinism test.** Build the merged rpmdb twice from identical inputs in the same Bazel action and `cmp` the outputs. Belongs in the `rpmdb-merge` package, not at the image level — regressions caught at the source binary, before they propagate into every image's layer hash.

Tar wrapper around `rpmdb.sqlite` follows the canonical uid/gid/mode/mtime rules already applied to `content.tar`.

## Multi-vendor sourcing during transition

Image-by-image migration preserves the existing `_DISTROS` matrix axis. Senku images can compose layers from:

| Source | Use for |
|---|---|
| Debian sid (`@debian//pkg/arch`) | `static` (zero-by-exclusion); transitional state for other images while Hummingbird migration is in progress |
| Hummingbird (`@hummingbird//pkg/arch`) | Glibc-bearing images post-migration |
| Wolfi (`//oci/distroless/wolfi/busybox-static/arch`) | `busybox-static` for `*_debug` variants (existing, see commit `e176b66`) |

The matrix factory in [[oci/distroless/matrix.bzl]] doesn't know which package manager produced any given tar — it just composes them. The `_DISTROS` list axis is the migration switch.

## Threat model and fallbacks

| Risk | Defended? | Notes |
|---|---|---|
| Hummingbird sunsets the public repo | Partial | Lockfile + per-rpm SHA256 means existing builds remain reproducible from cached bytes indefinitely. New builds break; need to migrate to a successor (Red Hat may publish a continuity path, given the OSS posture). Plan B: snapshot the Hummingbird repo to our own infra ahead of any sunset signal. |
| Hummingbird restricts free access | Same | Same mechanism — lockfile decouples build determinism from upstream availability. |
| Grype/Trivy drop hummingbird provider coverage | Defended | The supply-chain claim doesn't depend on scanner support: bytes are still backported. Worst case our images stop *appearing* zero on those scanners; transparent VEX and the published SBOM still describe the underlying truth. |
| Compromised Hummingbird repo (malicious RPM substituted) | Defended | GPG signature verification at lock and build time. Repo metadata is also GPG-signed (per `repomd.xml.asc`); both layers must be subverted simultaneously. Plus per-rpm SHA256 in our lockfile means any drift fails the checksum compare. |
| Compromised Hummingbird GPG key | Not defended | Out of scope. Same posture as any apt/apk key compromise — trust root issue requiring a coordinated response. |
| Scanner DB ingests fresh hummingbird CVE that we haven't rebuilt against | Defended by rebuild cadence | Daily/weekly snapshot bump CI job (open question, see below) closes the window; lockfile gets bumped, images rebuild, scan-clean test asserts. |

## Operational considerations

**Rebuild cadence is the actual competitive moat.** Chainguard's zero-CVE story is *continuous rebuild against current upstream*. Senku must match that or the zero claim drifts. Empirically, Hummingbird's `repomd.xml` revision bumped from `1778835791` to `1778852516` within ~3 hours during ADR drafting on 2026-05-15 — multiple updates per day. Daily auto-PR cadence is therefore the right floor; weekly would miss most upstream changes and let the wontfix window reopen. Recommend daily `@hummingbird//:pin` job with `_cve_test_stale_*` enforcement and "scan still clean" as the merge gate. Tracks closely to the existing [[oci/distroless/debian.yaml]] cadence with the same machinery, just faster.

**Triple-scanner verification.** CI gates each image on grype + trivy + at least one third scanner (snyk or osv-scanner) reporting zero. Single-scanner verification has been shown to be insufficient by this same investigation; the upstream-feed-ingestion lag is a real risk and only diverges between vendors.

**SBOM and attestation pipeline unchanged.** The cosign/SLSA chain in [[oci/mirror_push.bzl]] composes around `oci_image` and doesn't depend on the package source. The same `image_sbom` rule emits CycloneDX from whatever package metadata the image carries; the only change is purl prefix (`pkg:rpm/.../...?distro=hummingbird-<rev>` instead of `pkg:deb/debian/...`).

## Migration order

1. Drop `libc-bin`/`mawk` from current Debian-`static` — independent of Hummingbird, immediate win: removes 3 glibc CVE entries from the wontfix list while still on the old supply chain. Buys reaction time.
2. `rules_rpm` skeleton (`hummingbird.install` module extension + `@hummingbird//:pin` tool + `hummingbird_install.json`) + `hummingbird-rpm-extract` and `rpmdb-merge` Go binaries — the new infrastructure.
3. **`static` migrates to Hummingbird first.** Smallest dep closure (3 upstream packages); no glibc to wrangle; exercises the rules_rpm path on the simplest case. Validates the whole pipeline (lockfile → rpm-extract → rpmdb fragment → composed layer → scanner-zero verification with `ID=hummingbird`) end-to-end before the harder migrations.
4. `cc` image migrated to Hummingbird — first glibc-bearing target. Validates the consumer-scanner-zero claim that motivates the whole ADR.
5. `bash`, `nginx`, `python` migrated one at a time, each behind the same scanner-zero gate.
6. `nodejs` distroless on Hummingbird — the marquee demo: ship a real distroless node, without npm or node-gyp, on the same glibc Hummingbird's own `hi/nodejs` ships. Tracer-bullet (shipped): nodejs.org prebuilt tarball on cc-hummingbird, ~95–145MB uncompressed per major (20/24/26), comparable to `hi/nodejs`'s 134MB. The ADR's earlier "30–50MB" estimate predated Node's growth and is unattainable from the `--enable-static-*` tarball regardless of stripping. Source-build with `--shared-{openssl,zlib,brotli,cares,nghttp2,libuv,zstd} --with-intl=system-icu` against Hummingbird's existing rpms (libicu alone unbundles ~30MB) is tracked in [#230](https://github.com/arkeros/senku/issues/230) and targets Chainguard-comparable ~60MB.
7. `java` distroless on Hummingbird — same shape as nodejs, with four decisions worth flagging up front because each looks like a smaller choice than it is.

    - **LTS-only policy (17 / 21 / 25).** Adoptium's LTS list is [8, 11, 17, 21, 25]; we ship the latest three. 8 and 11 are excluded — they're still in Adoptium's LTS catalogue but far down the EOL slope, and the per-quarter CPU rebuild promise has finite cycles. 26 is excluded as a non-LTS (six-month support window) — matches Google Distroless's `java/BUILD` posture. Auto-rolls forward: when JDK 29 ships in ~2027, 17 rolls off and 29 takes its place.

    - **Temurin tarballs, not the Hummingbird `java-25-openjdk-headless` RPM.** This is the one place in the migration where we deviate from the secdb-routed pattern. Hummingbird ships 21 + 25 but *not* 17, and enterprise Java is still overwhelmingly 17 — dropping 17 to keep the supply chain pure would be the tail wagging the dog. Mixing JRE sources (Hummingbird for 21/25, Temurin for 17) was costed and rejected: two extract pipelines, two SBOM component shapes, two test surfaces, two CVE-response playbooks — at v1 the maintenance tax exceeds the secdb-routing benefit. Better to be uniformly Temurin and pay the NVD-CPE matching cost across all three majors. Revisit when Hummingbird ships 17 or when a fourth Java consumer in this repo justifies the duplication.

    - **Headless-only.** `Adoptium-headless`-equivalent: no AWT/Swing/X11. Java server workloads (Spring, Quarkus, Kafka) don't need them; client GUIs don't run in distroless. The known foot-gun is `BufferedImage.drawString` — Apache POI, Apache PDFBox text rendering, ImageIO with custom fonts — silently NPEs at runtime without `libfontmanager`/`libfreetype`. Acknowledged and accepted: image-rendering workloads need the full JRE, not this image. Worth a callout in the published image description so a consumer doesn't spend half a day debugging a missing-font stack trace.

    - **CPE: `cpe:2.3:a:oracle:openjdk` (not `eclipse:temurin`).** Empirically verified, not chosen by branding: probing the same in-tree grype DB against `cpe:2.3:a:oracle:openjdk:17.0.0:*:*:*:*:*:*:*` returns 95 matches; `cpe:2.3:a:eclipse:temurin:17.0.0:*` returns 0. NVD catalogues OpenJDK advisories under the upstream-codebase vendor regardless of which distributor (Temurin, Corretto, Microsoft, Zulu) ships the bytes — Adoptium itself cites this vendor in their own security disclosures. Using the builder-name CPE would be silent-zero by another name. The companion `_cve_test_silent_zero` gate from [#461](https://github.com/arkeros/senku/pull/461) catches any SBOM component that's neither secdb-routable nor has a CPE, and forces every future Java consumer through this same probe-before-ship workflow.

    - **JDK in the `_debug` variant at /jre/ (same path as release JRE).** `kubectl exec ... -- /jre/bin/jstack 1` works against either variant: the release simply lacks the binary, debug supplies it. Operators don't have to special-case the variant in their debug-runbook command/args overrides. The JDK is a strict superset of the JRE (same `bin/java`, same `lib/server/libjvm.so`, plus `jdk.*` modules) so the entrypoint and SBOM component are unchanged — this is the same trick Google Distroless uses for its `java*-debug` variants and is the reason that pattern survives operationally.

## See also

- [[docs/research/deb-vs-apk-vs-oci-components.md]] — broader format/channel analysis; Option B (OCI-component overlay) was the prior recommendation; this ADR supersedes it for glibc-bearing images specifically while preserving its conclusions for non-glibc cases
- [[devtools/build/tools/wolfi-apk-extract/main.go]] — apk extraction analog; pattern reused for rpm
- [[oci/distroless/common/variables.bzl]] — `DEBIAN_WONTFIX_CVES` (to be renamed `WONTFIX_CVES[distro]` keyed map)
- [[oci/distroless/matrix.bzl]] — the distro-agnostic image composition factory
- [ADR 0006](0006-bazel-native-cosign-mirror-signing.md) — cosign signing chain (unchanged by this decision)
