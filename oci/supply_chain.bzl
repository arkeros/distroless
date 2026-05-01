"""Supply-chain hygiene for OCI images.

Replaces the aspect-driven syft+grype chain (`build --aspects=…` in .bazelrc)
with explicit, addressable build targets per image. Each component is a
regular rule so it's inspectable, query-able, and varies per image without
aspect_hints side-channels.
"""

load("@grype.bzl//grype:defs.bzl", "grype_scan", "grype_test")
load("@supply_chain_tools//sbom:cyclonedx.bzl", "cyclonedx")
load("@supply_chain_tools//sbom:sbom.bzl", "sbom")

_JQ_TOOLCHAIN_TYPE = "@jq.bzl//jq/toolchain:type"

# Set difference: VEX'd CVE IDs minus CVEs the raw (no-VEX) scan still
# matches. Empty result = every VEX statement still silences something;
# non-empty = stale statements that should be deleted.
_VEX_STALE_JQ_FILTER = """
[.[].statements[]?.vulnerability.name] | unique - ([$scan[0].matches[]?.vulnerability.id] | unique) |
if length == 0 then "PASS: every VEX statement still suppresses a scan match" | halt_error(0)
else "FAIL: VEX statements with no corresponding raw-scan match (stale, delete them): \\(.)" | halt_error(1)
end
""".strip()

def _vex_stale_test_impl(ctx):
    jq_bin = ctx.toolchains[_JQ_TOOLCHAIN_TYPE].jqinfo.bin
    scan = ctx.file.raw_scan
    vex_files = ctx.files.vex

    runner = ctx.actions.declare_file("{}_runner.sh".format(ctx.label.name))
    ctx.actions.write(
        output = runner,
        content = """#!/bin/sh
exec {jq} --slurpfile scan {scan} -s '{filter}' {vex}
""".format(
            jq = jq_bin.short_path,
            scan = scan.short_path,
            filter = _VEX_STALE_JQ_FILTER,
            vex = " ".join([f.short_path for f in vex_files]),
        ),
        is_executable = True,
    )

    runfiles = ctx.runfiles(files = [scan, jq_bin] + vex_files)
    return [DefaultInfo(executable = runner, runfiles = runfiles)]

vex_stale_test = rule(
    implementation = _vex_stale_test_impl,
    test = True,
    attrs = {
        "raw_scan": attr.label(
            mandatory = True,
            allow_single_file = [".json"],
            doc = "Grype scan JSON produced WITHOUT --vex applied. Compare against " +
                  "VEX statements to detect ones that silence nothing — typically " +
                  "because the scanner's DB caught up to the distro tracker.",
        ),
        "vex": attr.label_list(
            mandatory = True,
            allow_files = [".json"],
            doc = "VEX documents whose statements should each correspond to a real " +
                  "scan match. Otherwise the statement is stale.",
        ),
    },
    toolchains = [config_common.toolchain_type(_JQ_TOOLCHAIN_TYPE, mandatory = True)],
    doc = "Test that fails when a VEX document carries statements for CVEs that " +
          "the raw (no-VEX) grype scan does not flag — i.e. statements that " +
          "would silence nothing. Mirrors `_cve_test_stale_ignores` for VEX.",
)

def image_supply_chain(image, fail_on_severity = "high", ignore_cves = None, vex = None, database = "@grype_database"):
    """Attach SBOM + CVE scan + policy test to an OCI image.

    Generates three targets, named after `image`'s base label:
        <base>_sbom         — CycloneDX 1.6 JSON, sourced from the build graph.
        <base>_cve_scan     — grype JSON report (artifact only).
        <base>_cve_test     — test target, gates on `fail_on_severity`.

    When `vex` is non-empty, two more:
        <base>_cve_scan_raw — grype JSON without --vex applied.
        <base>_vex_stale    — fails if any VEX statement silences nothing.

    Args:
      image: Label of the OCI image. Must be reachable from supply_chain_tools'
        gather_metadata aspect — its transitive deps must carry
        `PackageMetadataInfo` (Go modules via gazelle, .deb via rules_distroless's
        package_metadata patch).
      fail_on_severity: Threshold for `<base>_cve_test`. Default "high".
      ignore_cves: List of CVE IDs to allow-list. None = no allow-list.
        Prefer `vex` for anything that has a defensible justification; reserve
        `ignore_cves` for unjustifiable noise (e.g. distro wontfix).
      vex: List of OpenVEX document labels (see //oci:vex.bzl). Statements
        with status=not_affected or fixed remove matching results from the
        scan before `_cve_test` evaluates severity. Pairs with a stale-test
        that fires when grype's DB catches up and the statement becomes
        a no-op.
      database: Grype vulnerability DB target. Default `@grype_database`.
    """
    base = image.rsplit(":", 1)[-1]

    sbom(name = base + "_sbom_raw", target = image)
    cyclonedx(name = base + "_sbom", sbom = ":" + base + "_sbom_raw")
    grype_scan(
        name = base + "_cve_scan",
        database = database,
        sbom = ":" + base + "_sbom",
        vex = vex,
    )
    grype_test(
        name = base + "_cve_test",
        fail_on_severity = fail_on_severity,
        ignore_cves = ignore_cves,
        scan_result = ":" + base + "_cve_scan",
    )

    if vex:
        grype_scan(
            name = base + "_cve_scan_raw",
            database = database,
            sbom = ":" + base + "_sbom",
        )
        vex_stale_test(
            name = base + "_vex_stale",
            raw_scan = ":" + base + "_cve_scan_raw",
            vex = vex,
        )
