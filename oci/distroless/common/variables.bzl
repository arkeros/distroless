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

# Debian "no-DSA, Minor issue" CVEs that Debian Security has triaged but not
# yet backported a fix for. We track Debian unstable (sid) so most fixes flow
# in via lockfile bumps; this list is what's still pending upstream as of the
# current snapshot. Companion `_cve_test_stale_ignores` test fails when any
# entry vanishes from the scan, forcing us to delete it.
DEBIAN_WONTFIX_CVES = [
    # glibc (libc6) — all currently unfixed in sid 2.42-15
    "CVE-2026-5435",
    "CVE-2026-5450",
    "CVE-2026-5928",
]

# Busybox: only present in `*_debug_*` variants via `static_debug_layers`.
# Apply via `distroless_matrix(debug_ignore_cves = ...)`. Currently empty;
# the previous entries are all fixed in sid's busybox 1.37.0-10.1.
BUSYBOX_WONTFIX_CVES = []
