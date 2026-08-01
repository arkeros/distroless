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

load("//oci:vex.bzl", "vex_statement")

# glibc CVEs fixed in libc6 / libc-gconv-modules-extra at sid versions.
GLIBC_FIXED_VEX_STATEMENTS = []

# busybox CVEs fixed in 1.37.0-7 / 1.37.0-10.1. Only present in
# `*_debug_*` image variants via the busybox layer.
BUSYBOX_FIXED_VEX_STATEMENTS = []

# ncurses (libtinfo6) CVEs fixed in 6.6+20251231-1+. Present in any image
# that ships bash / readline-using tools.
NCURSES_FIXED_VEX_STATEMENTS = []

# nginx CVEs fixed upstream in the nginx.org .deb we actually ship, but
# still flagged because grype resolves the package against Debian's own
# `nginx` source package rather than nginx.org's.
#
# rules_distroless hardcodes `pkg:deb/debian/{name}` in `_deb_purl` with
# no vendor override, so every apt-sourced package routes to
# `debian:distro:debian:unstable` regardless of which repo supplied it.
# On the rpm side the same problem is solved structurally, by setting
# `purl_namespace = "nginx.org"` on `rpm.install` (see
# //bazel/include/oci.MODULE.bazel) — that qualifier pushes grype to
# NVD-CPE upstream-version matching. Until the apt extension grows the
# equivalent knob, the deb side needs scanner-side suppression here.
#
# Shared by //oci/distroless/nginx and by every frontend image built on
# the nginx base (apps/*), all of which ship the same nginx.org .deb.
NGINX_FIXED_VEX_STATEMENTS = [
    # Heap buffer overflow when a `map` directive uses regex matching and
    # a string expression references the map's capture variables before
    # the map output variable. nginx.org's advisory lists 1.30.4+ and
    # 1.31.3+ as not vulnerable; our stable pin is 1.30.4-1~trixie and
    # mainline is 1.31.3-1~trixie, both of which carry the fix. Debian's
    # tracker has it `<unfixed>` for *Debian's* nginx package, which is
    # what grype matches against.
    vex_statement(
        expires = "2026-09-15",
        impact_statement = "The .deb shipped here is nginx.org's 1.30.4-1~trixie (stable) / 1.31.3-1~trixie (mainline), both listed not-vulnerable in https://nginx.org/en/security_advisories.html. The match comes from Debian's tracker entry for Debian's own nginx source package, which grype selects because rules_distroless emits pkg:deb/debian/nginx with no vendor namespace.",
        products = ["pkg:deb/debian/nginx"],
        status = "fixed",
        vulnerability = "CVE-2026-42533",
    ),
]
