"""Reusable VEX statements for CVEs Debian has *not* fixed yet, where the
vulnerable code is demonstrably unreachable from the images that ship the
package. Sibling of debian_fixed_vex.bzl (grype lagging Debian) and of
variables.bzl's `*_WONTFIX_CVES` (bare ignores, no justification).

Prefer a statement here over a `*_WONTFIX_CVES` entry whenever the
argument can be made per binary — a VEX document is what consumers get
attested alongside the image, an ignore list is only a test suppression.

Lifecycle is the same as the other lists: `_cve_test_stale_vex` fires the
moment the scanner stops flagging the CVE (Debian ships the fix, or the
package drops out of the image), and the statement gets deleted.
"""

load("//oci:vex.bzl", "vex_statement")

# zlib1g 1:1.3.dfsg+really1.3.2-3 — CVE-2026-85091, High, unfixed in sid.
# Heap buffer overflow in gz_vacate() when gzprintf()/gzvprintf() run after
# a stalled non-blocking gzwrite(): the gzFile write API.
#
# zlib1g is in every Debian image built on cc because openssl 3.6.4-1's
# libcrypto.so.3 has DT_NEEDED entries for libz.so.1 and libzstd.so.1
# (Debian enabled zlib and zstd compression), and the cc layer ships both so
# libcrypto can load — see //images/cc:test_tls.yaml. bash reaches libcrypto
# through rust-coreutils; nginx links libcrypto and libz itself.
#
# Only two ELF objects in any of these images import zlib, and both use the
# streaming API alone (checked with `strings` on the .debs; dpkg-shlibdeps
# confirms nothing else Depends on zlib1g directly):
#   libcrypto.so.3  deflateInit_/deflate/deflateEnd,
#                   inflateInit_/inflate/inflateEnd
#   nginx           deflateInit2_/deflate/deflateEnd,
#                   inflateInit2_/inflate/inflateReset/inflateEnd
#                   (ngx_http_gzip_filter_module, ngx_http_gunzip_filter_module)
# No gz* symbol anywhere, so gz_vacate() is unreachable. Drops when Debian
# ships upstream fix madler/zlib e3dc0a85.
ZLIB_UNFIXED_VEX_STATEMENTS = [
    vex_statement(
        expires = "2026-12-01",
        impact_statement = "Only libcrypto.so.3 and nginx import zlib, and both use the streaming deflate/inflate API exclusively; no gz* (gzFile) symbol is imported anywhere in the image, so the gz_vacate() path is never reached.",
        justification = "vulnerable_code_not_in_execute_path",
        products = ["pkg:deb/debian/zlib1g"],
        status = "not_affected",
        vulnerability = "CVE-2026-85091",
    ),
]
