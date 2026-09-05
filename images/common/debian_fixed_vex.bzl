"""Reusable VEX statements for CVEs Debian has fixed in package versions
that grype's vulnerability DB hasn't synced yet.

Currently empty: with `distro=debian-unstable` in the apt.install PURL
qualifier, grype consults Debian's unstable Security Tracker directly and
already drops the CVEs we used to silence here. The threading is kept in
place — `image_supply_chain(vex = [":vex"])`, `distroless_matrix(debug_vex
= [":debug_vex"])`, the per-image `vex_document` targets — so the moment
grype's tracker matching ever falls back to NVD-only data (or a future CVE
needs scanner-side suppression), adding a statement is one line.

The companion `_vex_stale` test (in supply_chain.bzl) fires when a
statement here outlives the scanner's fix sync — i.e. silences nothing.
That's how this list gets pruned: stale tests turn red, statements get
deleted.

Statement-name conventions (`<package>_FIXED_VEX_STATEMENTS`) match how
each image composes them: cc/static/nginx pull glibc + busybox; bash adds
ncurses on top.
"""

# glibc CVEs fixed in libc6 / libc-gconv-modules-extra at sid versions.
GLIBC_FIXED_VEX_STATEMENTS = []

# busybox CVEs fixed in 1.37.0-7 / 1.37.0-10.1. Only present in
# `*_debug_*` image variants via the busybox layer.
BUSYBOX_FIXED_VEX_STATEMENTS = []

# ncurses (libtinfo6) CVEs fixed in 6.6+20251231-1+. Present in any image
# that ships bash / readline-using tools.
NCURSES_FIXED_VEX_STATEMENTS = []

# nginx, per channel. Empty: the nginx.org .deb is identified in the SBOM
# by its upstream version and an f5:nginx CPE (`upstream_identities` on
# its apt.install in //bazel/include/oci.MODULE.bazel), so grype matches
# it against NVD's upstream
# ranges rather than Debian's tracker. That retired the statement that used
# to live here — CVE-2026-42533, fixed upstream in 1.30.4 but flagged
# because Debian's own fix, 1.30.4-3, sorts above nginx.org's
# 1.30.4-1~trixie — and Debian-only entries such as CVE-2013-0337, which
# never reach an NVD-CPE match. Keyed by channel because the two pin
# different upstream versions; `_cve_test_stale_vex` prunes whatever
# stops matching.
#
# Shared by //images/nginx and by every frontend image built on the nginx
# base, all of which ship the same nginx.org .deb.
NGINX_FIXED_VEX_STATEMENTS = {
    "stable": [],
    "mainline": [],
}
