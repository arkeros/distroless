"common variables"

def quote(str):
    return '''"{}"'''.format(str)

OS_RELEASE = dict(
    PRETTY_NAME = "Distroless",
    NAME = "Debian GNU/Linux",
    ID = "debian",
    VERSION_ID = "{VERSION}",
    VERSION = "Debian GNU/Linux {VERSION} ({CODENAME})",
    HOME_URL = "https://github.com/arkeros/astrograde",
    SUPPORT_URL = "https://github.com/arkeros/astrograde/blob/main/distroless/README.md",
    BUG_REPORT_URL = "https://github.com/arkeros/astrograde/issues/new",
)

NOBODY = 65534
NONROOT = 65532
ROOT = 0

USER_IDS = {
    "root": ROOT,
    "nonroot": NONROOT,
}

MTIME = "0"

DEBUG_MODE = ["", "_debug"]
USERS = ["root", "nonroot"]

COMPRESSION = "zstd"

# Debian 13 (trixie) glibc CVEs flagged by Debian Security Tracker as
# "wontfix" — Debian has triaged them and will not backport upstream patches.
# Every image based on debian-13 inherits these from libc6/glibc, so the
# allow-list lives here rather than per-image. The companion
# `_cve_test_stale_ignores` test fails if any of these vanish from the scan,
# so we'll notice if upstream ever ships a fix.
# Common (universal: every Debian-13 image transitively includes glibc).
DEBIAN13_WONTFIX_CVES = [
    # glibc (libc6)
    # CVE-2026-4046: silenced via VEX (//oci/distroless/common:debian13_vex)
    # because the vulnerable code path (IBM1390/IBM1399 gconv modules) is
    # stripped at build time — see GLIBC_STRIPPED_GCONV.
    "CVE-2026-4437",
    "CVE-2026-5435",
    "CVE-2026-5450",
    "CVE-2026-5928",
]

# Busybox: only present in `*_debug_*` variants via `static_debug_layers`.
# Apply via `distroless_matrix(debug_ignore_cves = ...)`.
BUSYBOX_WONTFIX_CVES = [
    "CVE-2023-39810",
    "CVE-2026-26157",
    "CVE-2026-26158",
]

# gconv modules stripped from libc6 in every layer that ships it. Stripping
# turns the VEX claim from `vulnerable_code_not_in_execute_path` (narrative,
# fragile) into `vulnerable_code_not_present` (mechanically checkable).
#
# IBM1390/IBM1399: EBCDIC code pages for Japanese mainframe interop. Source of
# CVE-2026-4046 (iconv() assertion-failure DoS). Nothing in our stack iconv()s
# through them.
GLIBC_STRIPPED_GCONV = [
    "**/gconv/IBM1390.so",
    "**/gconv/IBM1399.so",
]
