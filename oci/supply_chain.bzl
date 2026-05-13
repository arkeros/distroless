"""Supply-chain hygiene for OCI images.

Replaces the aspect-driven syft+grype chain (`build --aspects=…` in .bazelrc)
with explicit, addressable build targets per image. Each component is a
regular rule so it's inspectable, query-able, and varies per image without
aspect_hints side-channels.
"""

load("@grype.bzl//grype:defs.bzl", "grype_scan", "grype_test")
load("@jq.bzl//jq:jq.bzl", "jq")
load("@supply_chain_tools//sbom:cyclonedx.bzl", "cyclonedx")
load("@supply_chain_tools//sbom:sbom.bzl", "sbom")

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
    # file path. A package shipped through multiple flatten layers (e.g.
    # libc6 in both the cc base and the nginx layer) ends up as duplicate
    # components with identical purls. Collapse by purl so consumers see one
    # row per distinct package; ordering is sorted-by-purl, deterministic.
    #
    # The same tool synthesizes `component.name` as `<purl-namespace>/<purl-name>`
    # ("debian/nginx" rather than "nginx"), which prevents grype's dpkg matcher
    # from doing an exact-direct-match against Debian's Security Tracker:
    # syft reads the prefixed name into `pkg.Name`, the tracker has no
    # "debian/nginx" entry, and the `upstream=<source>` fallback only kicks in
    # when source != name (grype's handleDefaultUpstream filters
    # name-equals-upstream qualifiers — package.go:492). Strip the prefix so
    # source==name packages (nginx, bash, busybox, sqlite3, ...) get the same
    # tracker coverage that `grype <image>` already gives consumers via
    # /var/lib/dpkg/status.d.
    jq(
        name = base + "_sbom",
        srcs = [":" + base + "_sbom_predupe"],
        out = base + "_sbom.json",
        filter = '.components |= (map(.name |= sub("^debian/"; "")) | unique_by(.purl))',
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
