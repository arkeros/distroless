"""Reusable VEX statements for CVEs fixed in the Hummingbird-sourced rpms
we ship, but still flagged by grype's Hummingbird secdb matching.

Hummingbird counterpart to //images/common:debian_fixed_vex.bzl.
The failure mode here is different from the Debian side: rather than a
missing vendor namespace, the mismatch is cross-vendor *release tags*.
//bazel/include/oci.MODULE.bazel sources nginx from nginx.org's own RHEL10
repo (with `purl_namespace = "nginx.org"`), while Hummingbird's secdb
advisories name Hummingbird's own rebuilds as the fixed version. rpmvercmp
then ranks e.g. `1.el10.ngx` below `2.hum1` and reports a fix gap that the
binary doesn't actually have.

The companion `_cve_test_stale_vex` test (in supply_chain.bzl) fires when a
statement here outlives the scanner's fix sync — i.e. silences nothing.
That's how this list gets pruned: stale tests turn red, statements get
deleted. Four nginx statements (CVE-2026-9256, CVE-2026-42055,
CVE-2026-60005, CVE-2026-42533) were removed exactly that way.

Statement-name conventions (`<package>_HUMMINGBIRD_FIXED_VEX_STATEMENTS`)
mirror the Debian module's `<package>_FIXED_VEX_STATEMENTS`.
"""

load("//oci:vex.bzl", "vex_statement")

# openssl-libs 3.5.9-0.5.hum1. CVE-2026-124709 (QUIC server: unbounded
# memory retention for rejected streams, OpenSSL advisory 2026-11-17) is
# fixed in Hummingbird's openssl 3.5.9-0.5.hum1 — Red Hat's CVE page lists
# `openssl-main-3.5.9-0.5.hum1` as the released fix — but grype's Hummingbird
# record still carries no fixed version, so it matches the patched build.
# Delete this when the feed learns the fix (`_cve_test_stale_vex` fires).
# Shared by every Hummingbird image that ships openssl-libs: all but static.
OPENSSL_HUMMINGBIRD_FIXED_VEX_STATEMENTS = [
    vex_statement(
        expires = "2027-01-31",
        impact_statement = "Fixed in Hummingbird openssl 3.5.9-0.5.hum1 (Red Hat lists openssl-main-3.5.9-0.5.hum1 as the fix); the image ships that build. grype's Hummingbird record for the openssl source package has no fixed version yet.",
        products = ["pkg:rpm/hummingbird/openssl-libs"],
        status = "fixed",
        vulnerability = "CVE-2026-124709",
    ),
]

# gcc runtime. Empty: the last statement, CVE-2026-95619 (integer overflow
# in libstdc++'s aligned `operator new`, fixed in Hummingbird gcc
# 16.2.1-2.hum1 but matched by a rangeless second record for the gcc source
# package), stopped matching with the 2026-11-24 grype database, which
# dropped that record. Shared by every Hummingbird image built on cc; static
# has no C++ runtime and gets no copy.
GCC_HUMMINGBIRD_FIXED_VEX_STATEMENTS = []

# nginx, per channel. Empty: nginx.org's rpm is identified by its upstream
# version and an f5:nginx CPE (`upstream_identities` on its rpm.install in
# //bazel/include/oci.MODULE.bazel), in the attested SBOM and on the image
# alike — the image's rpmdb carries no row for it, so a scanner reads the
# version out of the binary instead. Both scans match NVD's upstream ranges
# rather than Hummingbird's secdb, which retired the two statements that
# used to live here: CVE-2026-60005 and CVE-2026-42533, both fixed upstream
# in 1.30.4 but flagged because rpmvercmp ranked nginx.org's `1.el10.ngx`
# release tag below Hummingbird's own `2.hum1` rebuild. Keyed by channel
# because the two pin different upstream versions; `_cve_test_stale_vex`
# prunes whatever stops matching.
#
# Shared by //images/nginx and by every frontend image built on the
# hummingbird nginx base, all of which ship the same nginx.org rpm.
NGINX_HUMMINGBIRD_FIXED_VEX_STATEMENTS = {
    "stable": [],
    "mainline": [],
}
