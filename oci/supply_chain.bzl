"""Supply-chain hygiene for OCI images.

Replaces the aspect-driven syft+grype chain (`build --aspects=…` in .bazelrc)
with explicit, addressable build targets per image. Each component is a
regular rule so it's inspectable, query-able, and varies per image without
aspect_hints side-channels.
"""

load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@grype.bzl//grype:defs.bzl", "grype_scan", "grype_test")
load("@jq.bzl//jq:jq.bzl", "jq")
load("@supply_chain_tools//sbom:cyclonedx.bzl", "cyclonedx")
load("@supply_chain_tools//sbom:sbom.bzl", "sbom")

# Components are routable by grype iff their purl prefix matches one of
# grype's ecosystem matchers (rpm/deb/apk for distro secdb; golang/npm/
# pypi/maven/gem/cargo/nuget/hex/bitnami/alpm for language/OS ecosystems
# via GHSA + provider DBs) OR they carry an explicit `cpe` field for the
# `stock` NVD-CPE matcher.
#
# Anything else — notably `pkg:generic/...` (upstream-binary deps like
# nodejs.org's prebuilt node tarball) and `pkg:github/...` (GitHub release
# downloads) — has no grype matcher and is a silent-zero hazard.
# Empirically verified: Node 18.0.0 produces 49+ grype matches with `cpe:`
# set, 0 without (pkg:generic/ no-match); `pkg:golang/golang.org/x/net@v0.15.0`
# produces 5 GHSA matches without cpe (golang matcher routes via GHSA).
# See ADR 0007's disqualification of SUSE/`bci-micro:16.0` and AlmaLinux
# `ID_LIKE` for the same fraud-by-silence anti-pattern.
#
# The matcher list is from grype/matcher/matchers.go's NewDefaultMatchers;
# keep in lock-step if grype gains/drops matchers (cf. //bazel/modules/grype).
_SILENT_ZERO_FILTER = """
.components | map(
  select(
    ((.purl // "") | test("^pkg:(rpm|deb|apk|alpm|bitnami|cargo|gem|golang|hex|maven|npm|nuget|pypi)/") | not) and
    ((.cpe // "") == "")
  )
) | map({purl, name})
""".strip()

def image_sbom(image):
    """Attach a CycloneDX SBOM to an OCI image, without CVE scanning or gating.

    Lighter-weight counterpart to `image_supply_chain` for cases where the SBOM
    is the only supply-chain artifact needed (e.g. the index target of a
    multi-arch image, where per-arch CVE testing already happens via `oci_image`
    and the index just needs a unified SBOM for `mirror_push`'s SBOM attestation).

    Generates `<base>_sbom` (CycloneDX 1.6 JSON) named after `image`'s base label.

    Args:
      image: Label of the OCI image. Same reachability requirements as
        `image_supply_chain` — transitive deps must carry `PackageMetadataInfo`.
    """
    base = image.rsplit(":", 1)[-1]
    sbom(name = base + "_sbom_raw", target = image)
    cyclonedx(name = base + "_sbom_predupe", sbom = ":" + base + "_sbom_raw")

    # supply_chain_tools' cyclonedx tool emits one component per
    # `PackageMetadataInfo`-bearing target it walks, deduping only by metadata
    # file path. Two cleanup passes here:
    #
    # 1. A package shipped through multiple flatten layers (e.g. libc6 in
    #    both the cc base and the nginx layer) ends up as duplicate components
    #    with identical purls. Collapse by purl so consumers see one row per
    #    distinct package; sorted-by-purl ordering is deterministic.
    #
    # 2. The cyclonedx tool synthesizes `component.name` as
    #    `<purl-namespace>/<purl-name>` ("debian/nginx" rather than "nginx",
    #    "hummingbird/glibc" rather than "glibc"), which prevents grype's
    #    dpkg/rpm matcher from doing an exact-direct-match against the
    #    Security Tracker secdb: syft reads the prefixed name into `pkg.Name`,
    #    the tracker has no "debian/nginx" or "hummingbird/glibc" entry, and
    #    the `upstream=<source>` fallback only kicks in when source != name
    #    (grype's handleDefaultUpstream filters name-equals-upstream
    #    qualifiers — package.go:492). Strip the prefix on rpm+deb components
    #    so source==name packages (nginx, bash, busybox, glibc, ...) get the
    #    same tracker coverage that `grype <image>` already gives consumers
    #    via /var/lib/dpkg/status.d or /usr/lib/sysimage/rpm/rpmdb.sqlite.
    #
    # Note: rules_rpm tool-side Go module deps don't need filtering here —
    # `gather_metadata` is taught to skip them via
    # //bazel/patches:supply_chain_tools_rule_filters_rpm.patch.
    jq(
        name = base + "_sbom",
        srcs = [":" + base + "_sbom_predupe"],
        out = base + "_sbom.json",
        filter = '.components |= (map(if (.purl // "") | test("^pkg:(rpm|deb)/") then .name |= sub("^[^/]+/"; "") else . end) | unique_by(.purl))',
    )

def image_supply_chain(image, fail_on_severity = "high", ignore_cves = None, vex = None, database = "@grype_database"):
    """Attach SBOM + CVE scan + policy test to an OCI image.

    Generates the following targets, named after `image`'s base label:
        <base>_sbom               — CycloneDX 1.6 JSON, sourced from the build graph.
        <base>_cve_scan           — grype JSON report (artifact only).
        <base>_cve_test           — gates on `fail_on_severity`.
        <base>_cve_test_stale_ignores
                                  — fails when an `ignore_cves` entry no
                                    longer matches a scan CVE.
        <base>_cve_test_stale_vex (only when `vex` is non-empty)
                                  — fails when a VEX statement targets a
                                    CVE the scanner doesn't flag.

    Args:
      image: Label of the OCI image. Must be reachable from supply_chain_tools'
        gather_metadata aspect — its transitive deps must carry
        `PackageMetadataInfo` (Go modules via gazelle, .deb via rules_distroless's
        package_metadata patch).
      fail_on_severity: Threshold for `<base>_cve_test`. Default "high".
      ignore_cves: List of CVE IDs to allow-list (flat). Prefer `vex` for
        anything with a defensible justification.
      vex: List of OpenVEX 0.2.0 document labels (see //oci:vex.bzl).
        grype.bzl extracts each statement's CVE ID at action time and adds
        it to the suppression set. The companion `_stale_vex` test fires
        when a statement no longer matches a scan CVE.
      database: Grype vulnerability DB target. Default `@grype_database`.
    """
    base = image.rsplit(":", 1)[-1]

    image_sbom(image = image)
    grype_scan(
        name = base + "_cve_scan",
        database = database,
        sbom = ":" + base + "_sbom",
    )
    grype_test(
        name = base + "_cve_test",
        fail_on_severity = fail_on_severity,
        ignore_cves = ignore_cves,
        scan_result = ":" + base + "_cve_scan",
        vex = vex,
    )

    # Silent-zero gate. Fails when the SBOM carries components that grype
    # has no matcher for — see _SILENT_ZERO_FILTER above for the rationale.
    # This is the structural guard for ADR 0007's "no fraud-by-silence"
    # claim: a CVE scan that returns zero is only meaningful when every
    # component is actually being checked. Without this, a nodejs.org
    # tarball without `cpe` would pass `_cve_test` while silently skipping
    # every Node CVE in the world.
    #
    # Decomposed `jq` + `diff_test` instead of `jq_test` because jq_test's
    # error-message templating breaks on filters containing `""` and `(`,
    # which any non-trivial routability check inevitably has.
    jq(
        name = base + "_silent_zero_violations",
        srcs = [":" + base + "_sbom"],
        out = base + "_silent_zero_violations.json",
        filter = _SILENT_ZERO_FILTER,
    )
    write_file(
        name = base + "_silent_zero_expected",
        out = base + "_silent_zero_expected.json",
        # Trailing empty string forces a final newline so diff_test matches
        # jq's default trailing-newline output.
        content = ["[]", ""],
    )
    diff_test(
        name = base + "_cve_test_silent_zero",
        file1 = ":" + base + "_silent_zero_expected",
        file2 = ":" + base + "_silent_zero_violations",
        failure_message = "SBOM contains components with neither a secdb-routable purl (pkg:rpm|deb|apk) nor a `cpe` field. See //bazel/patches:package_metadata_cpe.patch for how to attach `cpe=...` to a `package_metadata` target.",
    )
