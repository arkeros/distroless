"""VEX statements for CVEs Debian has fixed but grype's DB hasn't synced.

Grype matches by NVD's vulnerable-version range, which doesn't reflect
distro-level backports. Debian's Security Tracker calls each of these
"fixed" in the package version we pull from sid, but grype keeps flagging
them. `fixed` VEX statements tell `grype --vex` to suppress the match.

Drop entries here when grype's DB syncs the upstream/distro fix — confirm
by removing the statement and watching the cve_test fail (or not) on a
fresh DB. Each statement carries the fix version in `impact_statement`
so a future reader can verify against current package state.
"""

load("//oci:vex.bzl", "vex_statement")

# glibc CVEs fixed in libc6 2.42-14 / 2.42-15.
# Both `libc6` and `libc-gconv-modules-extra` ship from the same `glibc`
# source; grype reports against both binary packages.
GLIBC_FIXED_VEX_STATEMENTS = [
    vex_statement(
        vulnerability = "CVE-2026-4046",
        products = [
            "pkg:deb/debian/libc6",
            "pkg:deb/debian/libc-gconv-modules-extra",
        ],
        status = "fixed",
        impact_statement = (
            "Fixed in glibc 2.42-15 (Debian source). NVD's vulnerable-range " +
            "data hasn't been updated and grype's DB lags accordingly."
        ),
    ),
    vex_statement(
        vulnerability = "CVE-2026-4437",
        products = [
            "pkg:deb/debian/libc6",
            "pkg:deb/debian/libc-gconv-modules-extra",
        ],
        status = "fixed",
        impact_statement = (
            "Fixed in glibc 2.42-14+ (Debian source). Grype DB lag."
        ),
    ),
]

# busybox CVEs fixed in 1.37.0-7 / 1.37.0-10.1.
# Only present in `*_debug_*` image variants via the busybox layer.
BUSYBOX_FIXED_VEX_STATEMENTS = [
    vex_statement(
        vulnerability = "CVE-2023-39810",
        products = ["pkg:deb/debian/busybox-static"],
        status = "fixed",
        impact_statement = "Fixed in busybox 1.37.0-7+ (Debian). Grype DB lag.",
    ),
    vex_statement(
        vulnerability = "CVE-2026-26157",
        products = ["pkg:deb/debian/busybox-static"],
        status = "fixed",
        impact_statement = "Fixed in busybox 1.37.0-10.1+ (Debian). Grype DB lag.",
    ),
    vex_statement(
        vulnerability = "CVE-2026-26158",
        products = ["pkg:deb/debian/busybox-static"],
        status = "fixed",
        impact_statement = "Fixed in busybox 1.37.0-10.1+ (Debian). Grype DB lag.",
    ),
]

# ncurses (libtinfo6) CVE fixed in 6.6+20251231-1.
# Present in any image that ships bash / readline-using tools.
NCURSES_FIXED_VEX_STATEMENTS = [
    vex_statement(
        vulnerability = "CVE-2025-69720",
        products = ["pkg:deb/debian/libtinfo6"],
        status = "fixed",
        impact_statement = "Fixed in ncurses 6.6+20251231-1+ (Debian). Grype DB lag.",
    ),
]

# Convenience: every statement, applied to images that pull mixed packages.
# Statements that don't match a scanned package are silent no-ops.
DEBIAN_FIXED_VEX_STATEMENTS = (
    GLIBC_FIXED_VEX_STATEMENTS +
    BUSYBOX_FIXED_VEX_STATEMENTS +
    NCURSES_FIXED_VEX_STATEMENTS
)
