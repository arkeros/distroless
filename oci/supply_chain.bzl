"""Supply-chain hygiene for OCI images.

Replaces the aspect-driven syft+grype chain (`build --aspects=…` in .bazelrc)
with explicit, addressable build targets per image. Each component is a
regular rule so it's inspectable, query-able, and varies per image without
aspect_hints side-channels.
"""

load("@grype.bzl//grype:defs.bzl", "grype_scan", "grype_test")
load("@supply_chain_tools//sbom:cyclonedx.bzl", "cyclonedx")
load("@supply_chain_tools//sbom:sbom.bzl", "sbom")

def image_supply_chain(image, fail_on_severity = "high", ignore_cves = None, database = "@grype_database"):
    """Attach SBOM + CVE scan + policy test to an OCI image.

    Generates three targets, named after `image`'s base label:
        <base>_sbom        — CycloneDX 1.6 JSON, sourced from the build graph.
        <base>_cve_scan    — grype JSON report (artifact only).
        <base>_cve_test    — test target, gates on `fail_on_severity`.

    Args:
      image: Label of the OCI image. Must be reachable from supply_chain_tools'
        gather_metadata aspect — its transitive deps must carry
        `PackageMetadataInfo` (Go modules via gazelle, .deb via rules_distroless's
        package_metadata patch).
      fail_on_severity: Threshold for `<base>_cve_test`. Default "high".
      ignore_cves: List of CVE IDs to allow-list. None = no allow-list.
      database: Grype vulnerability DB target. Default `@grype_database`.
    """
    base = image.rsplit(":", 1)[-1]

    sbom(name = base + "_sbom_raw", target = image)
    cyclonedx(name = base + "_sbom", sbom = ":" + base + "_sbom_raw")
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
    )
