"""Providers carried by rules_rpm targets.

Shape mirrors @supply_chain_tools//tools/sbom:providers.bzl
(SbomInfo + TransitiveMetadataInfo) so consumers writing aspect-driven
composition (e.g. `rpmdb_merge_rule(gathering_aspect = ...)`) get the same
ergonomics as image_sbom.

- RpmHeaderInfo is the direct provider emitted by `rpm_package` rules — one
  per (package, arch). Carries the raw RPM general-header blob File plus
  enough identity metadata to route the blob into the rpmdb Packages row
  without re-parsing it during the merge.

- TransitiveRpmHeaderInfo is the rolled-up depset the aspect builds as it
  walks the image. Consumers read the `headers` depset to enumerate every
  RPM header reachable from the target.
"""

RpmHeaderInfo = provider(
    doc = "Per-package RPM general-header blob, attached to rpm_package targets.",
    fields = {
        "header": "File. Raw RPM general-header bytes (output of rpm-extract).",
        "package": "string. Package name, e.g. \"tzdata\".",
        "version": "string. Package version, e.g. \"2026a-1.1.hum1\".",
        "arch": "string. Package architecture, e.g. \"noarch\" or \"x86_64\".",
    },
)

TransitiveRpmHeaderInfo = provider(
    doc = "Rolled-up RPM headers gathered by gather_rpm_headers aspect.",
    fields = {
        "headers": "depset of RpmHeaderInfo structs.",
    },
)
