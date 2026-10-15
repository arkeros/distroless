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

# gcc runtime: libstdc++ / libgcc / libgomp / libatomic 16.2.1-3.1.hum1.
# CVE-2026-95619 (integer overflow in libstdc++'s aligned `operator new`)
# is fixed in Hummingbird's gcc 16.2.1-2.hum1 — RHSA-2026:58503, and Red
# Hat's CVE page lists `gcc-main-16.2.1-2.hum1` as the released fix — and
# grype's own Hummingbird record for the `gcc` source package says the
# same: `< 16.2.1-2.hum1`, state fixed. But the feed carries a second
# record for the same CVE and source package with no version range at
# all, and that one matches every binary built from gcc, whatever its
# version, as "affected, no fix". Its sibling CVE-2026-102010 has only the
# well-formed record (`< 16.2.1-3.hum1`) and stopped matching the moment
# the lockfile moved to 16.2.1-3. Expires the way every statement here
# does: re-check the feed, and delete this when the rangeless record is
# gone. Shared by every Hummingbird image built on cc; static has no C++
# runtime and gets no copy.
GCC_HUMMINGBIRD_FIXED_VEX_STATEMENTS = [
    vex_statement(
        expires = "2027-01-10",
        impact_statement = "Fixed in Hummingbird gcc 16.2.1-2.hum1 (RHSA-2026:58503); the image ships 16.2.1-3.1.hum1. grype matches a rangeless Hummingbird record for the gcc source package that contradicts the feed's own fixed-at-16.2.1-2.hum1 record for the same CVE.",
        products = [
            "pkg:rpm/hummingbird/libstdc++",
            "pkg:rpm/hummingbird/libgcc",
            "pkg:rpm/hummingbird/libgomp",
            "pkg:rpm/hummingbird/libatomic",
        ],
        status = "fixed",
        vulnerability = "CVE-2026-95619",
    ),
]

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
