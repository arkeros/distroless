# nginx is identified by its upstream, in the SBOM and on the image

**Status:** Accepted, 2026-09-05.

The nginx images ship nginx.org's own packages, and everything that describes
them now says so. In the **SBOM** the package is `pkg:generic/nginx@<upstream
version>` with the CPE `cpe:2.3:a:f5:nginx:<upstream version>`, not the
`pkg:deb/debian/nginx` or `pkg:rpm/nginx.org/nginx` of the archive it was
installed from. On the image there is no dpkg status entry and no rpmdb row
for it, so a scanner reading the filesystem reads the version out of the
binary and arrives at the same identity. The publication gate reads that
consumer's scan alongside the project's own, so the two views cannot drift
apart unnoticed. The mechanism is general — `upstream_identities` on
`apt.install` and `rpm.install` — and nginx is its only user.

## Why now

grype routes a package by the namespace of its purl. `pkg:deb/debian/…` goes
to Debian's security tracker, `pkg:rpm/…?distro=hummingbird-1` to
Hummingbird's secdb, and each tracker names its own rebuild as the fixed
version. nginx.org's build is neither of those rebuilds. Debian fixed
CVE-2026-42533 in its `1.30.4-3`, so nginx.org's `1.30.4-1~trixie`, which
carries the upstream fix, sorted below it and was flagged Critical.
Hummingbird's secdb named `1.30.4-2.hum1`, and rpmvercmp ranked nginx.org's
`1.el10.ngx` release tag below it, which flagged CVE-2026-42533 and
CVE-2026-60005 on a build that carried both fixes. Three **VEX statement**s
existed only to argue with someone else's version ordering, one per channel
and distro, each with a 90-day expiry to re-justify. That is a version
comparison the project was never entitled to win, restated every quarter.

The question that started the work was whether fetching nginx from its
GitHub release would give a `pkg:github` purl scanners could route. It would
not, and the answer to that question is the first decision below.

## Decisions

### Identity by upstream: a generic purl and a CPE, declared where the package is

Candidate identities were tested against the pinned grype and database, on
nginx 1.30.3, which has three known CVEs fixed in 1.30.4:

| Identity | grype | OSV |
|---|---|---|
| `pkg:github/nginx/nginx@release-1.30.3`, no CPE | 0 findings | empty |
| `pkg:generic/nginx@1.30.3`, no CPE | 0 findings | empty |
| either purl with `cpe:2.3:a:f5:nginx:1.30.3` | 3, fixed 1.30.4 and 1.31.3 | empty |
| `pkg:bitnami/nginx@1.30.3` | 3, same fixes | 4, one of them an NGINX Unit bug mis-mapped |
| any of the above at 1.30.4 | 0 | Bitnami: 1 |

grype knows `pkg:github` only as GitHub Actions and OSV has no GitHub
ecosystem, so that purl is a silent zero, the failure mode ADR 0007
disqualifies. The CPE route is the one Chainguard's apko SBOMs use for the
same package, and grype's stock matcher resolves it against NVD's upstream
ranges. `pkg:bitnami` would have served OSV too, at the price of naming a
vendor we do not ship and inheriting that feed's noise.

The identity is declared on the install tag, `upstream_identities = {"nginx":
"f5:nginx"}`, and the package extensions emit it: `rules_rpm` in-repo, and
`rules_distroless` through the patch this project already carries. The first
version rewrote the purl in a jq stage at the end of `image_sbom`, keyed on
`pkg:deb/debian/nginx`, which rules_distroless emits for any apt source. That
put a package-specific table in `oci/` and would have relabelled a
Debian-sourced nginx too. The version is the upstream one, epoch and distro
revision stripped, because that is what NVD ranges are expressed in; the
lockfiles record the exact archive installed.

### The image says the same: no distro metadata for such a package

Changing the SBOM alone fixed the project's scan and regressed the
consumer's. The image still shipped `/var/lib/dpkg/status.d/nginx` and, on
Hummingbird, an rpmdb row, because those are what make vanilla scanners work
on a distroless image at all — and there nginx was a distro package again.
Verified on the built layouts: `grype <image>` reported CVE-2026-42533 as
Critical with fix `1.30.4-3` on Debian, and all three July 2026 CVEs with
fix `1.30.4-2.hum1` on Hummingbird, while the attested **Scan record**
reported none.

Two ways out were built and one kept. Keeping the entries and restoring the
VEX statements for the consumer's benefit, with the gate taught to measure
their scan, was rejected: it ships an artifact that contradicts its own SBOM
and a document to explain the contradiction. Removing the entries was kept.
A package in `upstream_identities` contributes no dpkg status entry (the apt
template's `{statusd}` is empty for it) and no rpmdb row (`rpm_package` gets
`rpmdb = False`). With nothing to read, syft falls back to its `nginx-binary`
classifier, which takes the version from the `nginx version: nginx/…` string
in `/usr/sbin/nginx` and emits `pkg:generic/nginx@<version>` with the f5
CPE. The consumer's grype and the project's now report the same identity and
the same findings, none. Every other package keeps its entry.

This is a deliberate exception to ADR 0007's rule against silence. A
scanner with no nginx binary classifier — trivy today — sees no nginx on the
image and must read the attested SBOM, where it is listed. That is preferred
to the alternative, in which trivy reports a Critical the build does not
have and the project publishes a statement asking to be believed.

### The gate reads the consumer's scan too

`image_supply_chain` takes `image_scans`, and `distroless_matrix` passes each
**Index**'s first-architecture load tarball, scanned as `docker-archive`, when
its `consumer_scan` switch is on. The severity and stale gates then read the
union of the SBOM scan and that scan, so a package the image describes
differently from the SBOM, or a finding only a consumer would see, turns the
gate red rather than reaching the consumer. One architecture suffices: what
syft reads from dpkg status, the rpmdb or a binary does not vary by
architecture. The attested Scan record stays the SBOM scan, for the reason
ADR 0015 gives: it says what the scanner found in what was attested.

The switch is on for nginx and off everywhere else. It was first wired for
every family, which added about a minute and a half to a five-minute CI
test job and found nothing: for a family whose packages all carry their
distro's metadata the two views agree by construction. The scan earns its
cost where the image has been made to agree with the SBOM by hand, and that
is the family with an `upstream_identities` entry. A second such family
turns the switch on with its own image-identity test.

### Controls

Each thing this decision relies on has a test that fails when it stops being
true.

- `nginx_<channel>_<distro>_sbom_identity_test` diffs the SBOM's nginx
  identity against the lockfile's upstream version, so a channel bump cannot
  leave a stale identity.
- `nginx_<channel>_<distro>_image_identity_test` scans the image as a
  consumer would, in CycloneDX form, and diffs the same way, so the image and
  the SBOM cannot disagree.
- `nginx_cpe_canary_test` scans a fixed SBOM of nginx 1.30.3 against the
  pinned database and requires the three known CVEs to be found. NVD's own
  records lacked an open-source nginx CPE for two of them; Anchore's
  enrichment of the grype database supplied it, and the daily database bump
  now notices the day it stops.
- The container structure test asserts the dpkg entry is absent and that
  `nginx -v` prints the version string the classifier reads.
- The version-stripping helpers are unit-tested in both extensions.
- `grype_scan(image = …)` in grype.bzl failed at analysis for every target
  and fed OCI layouts to grype as docker archives; both fixed upstream in
  arkeros/grype.bzl `f4ca2e2`, which the override pins.

## Consequences

- The three nginx VEX statements are gone, and with them CVE-2013-0337's
  `not_affected` analysis, a Debian-only tracker entry that no longer reaches
  any scan. `NGINX_FIXED_VEX_STATEMENTS` and its Hummingbird twin stay wired,
  empty.
- The CPE route depends on Anchore's enrichment where NVD lags. Its failure
  is quiet, nothing found, which is why the canary exists and why the
  distro-tracker route, which failed loudly, was not simply the worse one.
- trivy reports no nginx on the image. The SBOM is the answer, and the README
  says so.
- Frontend images built on the nginx base inherit all of this, since they
  ship the same layer.
- `upstream_identities` is only safe for a package syft can identify from
  its binary. A second user of the attribute needs its own image-identity
  test before it needs anything else.
- The generic purl keeps `arch` as the archive named it, `amd64` on one
  distro and `x86_64` on the other, consistent with the other purls and no
  better.

## Revisit when

- rules_distroless learns a vendor namespace for apt sources, or grype's dpkg
  matcher learns to skip a tracker whose fixed version names a different
  build. Either would let nginx stay a deb on the image.
- syft's nginx classifier changes what it reads, or trivy gains one. The
  container test fails on the first; the second removes the exception.
- The canary goes red without a nginx release to explain it: the enrichment
  moved, and the identity needs another route.
- A second package wants `upstream_identities`. It brings its own
  image-identity test and turns `consumer_scan` on for its family.
- A family without `upstream_identities` diverges between image and SBOM
  for some other reason. The switch is then the wrong shape, and the scan
  should go back to every family.
