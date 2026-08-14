"""Reusable VEX statements for CVEs fixed in the Hummingbird-sourced rpms
we ship, but still flagged by grype's Hummingbird secdb matching.

Hummingbird counterpart to //oci/distroless/common:debian_fixed_vex.bzl.
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
deleted. Two predecessors of the nginx statements below (CVE-2026-9256 and
CVE-2026-42055) were removed exactly that way.

Statement-name conventions (`<package>_HUMMINGBIRD_FIXED_VEX_STATEMENTS`)
mirror the Debian module's `<package>_FIXED_VEX_STATEMENTS`.
"""

load("//oci:vex.bzl", "vex_statement")

# nginx CVEs fixed upstream in the nginx.org rpm we actually ship, but still
# flagged because grype compares its release tag against Hummingbird's own
# nginx rebuild.
#
# Shared by //oci/distroless/nginx and by every frontend image built on the
# hummingbird nginx base (apps/*), all of which ship the same nginx.org rpm.
#
# Keyed by nginx channel because the two channels pin different upstream
# versions, and only some of those fall below the advisory's fixed version.
# Same shape as NGINX_FIXED_VEX_STATEMENTS in the Debian module.
NGINX_HUMMINGBIRD_FIXED_VEX_STATEMENTS = {
    "stable": [
        # CVE-2026-60005 — uninitialized-memory disclosure in
        # ngx_http_slice_module. Fixed upstream in nginx 1.30.4 (stable) per
        # https://nginx.org/en/security_advisories.html, which is exactly the
        # version our stable lockfile pins.
        vex_statement(
            expires = "2026-09-15",
            impact_statement = "Fixed upstream in nginx 1.30.4; the el10.ngx rpm shipped here is that exact version. The Hummingbird secdb advisory targets Hummingbird's own nginx-1.30.4-2.hum1 build, and rpmvercmp ranks the nginx.org 1.el10.ngx release tag below 2.hum1 — a cross-vendor release-tag artifact, not a missing fix.",
            products = ["pkg:rpm/nginx.org/nginx"],
            status = "fixed",
            vulnerability = "CVE-2026-60005",
        ),
        # CVE-2026-42533 — heap buffer overflow when a `map` directive uses
        # regex matching and a string expression references the map's capture
        # variables before the map output variable. nginx.org's advisory lists
        # 1.30.4+ (stable) and 1.31.3+ (mainline) as not vulnerable, and our
        # stable lockfile pins exactly 1.30.4. Same release-tag artifact as
        # CVE-2026-60005 above, and the Debian twin of this statement lives in
        # NGINX_FIXED_VEX_STATEMENTS["stable"] — the two distros flag the same
        # nginx.org build for the same reason.
        vex_statement(
            expires = "2026-09-15",
            impact_statement = "Fixed upstream in nginx 1.30.4; the el10.ngx rpm shipped here is that exact version. The Hummingbird secdb advisory targets Hummingbird's own nginx-1.30.4-2.hum1 build, and rpmvercmp ranks the nginx.org 1.el10.ngx release tag below 2.hum1 — a cross-vendor release-tag artifact, not a missing fix.",
            products = ["pkg:rpm/nginx.org/nginx"],
            status = "fixed",
            vulnerability = "CVE-2026-42533",
        ),
    ],
    # Empty: mainline pins 1.31.3, which sorts above both advisories' fixed
    # versions, so grype stops matching on its own. A statement here would be
    # stale — `_cve_test_stale_vex` enforces that.
    "mainline": [],
}
