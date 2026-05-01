"""Reusable VEX statements for glibc CVEs that span multiple distroless images.

Statements are constants. Each image authors its own `vex_document` (so the
`@id`, author, and timestamp are owned by that image's audit trail) but
composes from these shared statements when the underlying claim — typically
"this CVE's vulnerable code path is stripped at build time" — is identical
across images.
"""

load("//oci:vex.bzl", "vex_statement")

# CVE-2026-4046: iconv() assertion-failure DoS via IBM1390/IBM1399 gconv
# modules. The vulnerable code path is stripped from libc6 at build time
# (see GLIBC_STRIPPED_GCONV in variables.bzl), so the code is not present
# in any image that ships our filtered libc6 layer.
GLIBC_STRIPPED_GCONV_VEX = vex_statement(
    vulnerability = "CVE-2026-4046",
    products = ["pkg:deb/debian/libc6"],
    status = "not_affected",
    justification = "vulnerable_code_not_present",
    impact_statement = (
        "The IBM1390 and IBM1399 gconv modules — the only paths into the " +
        "vulnerable iconv() code — are stripped from libc6 at build time " +
        "by tar_filter on each layer that ships glibc. See " +
        "GLIBC_STRIPPED_GCONV in oci/distroless/common/variables.bzl."
    ),
)
