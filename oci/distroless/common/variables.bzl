"common variables"

def quote(str):
    return '''"{}"'''.format(str)

DEBIAN_OS_RELEASE = dict(
    PRETTY_NAME = "Distroless",
    NAME = "Debian GNU/Linux",
    ID = "debian",
    VERSION_ID = "{VERSION}",
    VERSION = "Debian GNU/Linux {VERSION} ({CODENAME})",
    HOME_URL = "https://github.com/arkeros/senku",
    SUPPORT_URL = "https://github.com/arkeros/senku/blob/main/oci/distroless/README.md",
    BUG_REPORT_URL = "https://github.com/arkeros/senku/issues/new",
)

# Hummingbird-derived images. `ID=hummingbird` is the scanner-routing key
# (grype/trivy match exact-string only, no ID_LIKE fallback — see ADR 0007);
# NAME / PRETTY_NAME carry the senku brand for human readers. VERSION_ID is
# the Hummingbird snapshot revision (Unix timestamp from repomd.xml).
HUMMINGBIRD_OS_RELEASE = dict(
    PRETTY_NAME = "distroless.io (Hummingbird-derived)",
    NAME = "distroless.io",
    ID = "hummingbird",
    ID_LIKE = "rhel fedora",
    VERSION_ID = "{VERSION}",
    HOME_URL = "https://github.com/arkeros/senku",
    SUPPORT_URL = "https://github.com/arkeros/senku/blob/main/oci/distroless/README.md",
    BUG_REPORT_URL = "https://github.com/arkeros/senku/issues/new",
)

OS_RELEASE_BY_DISTRO = {
    "debian": DEBIAN_OS_RELEASE,
    "hummingbird": HUMMINGBIRD_OS_RELEASE,
}

# Back-compat for any caller that still imports OS_RELEASE directly.
OS_RELEASE = DEBIAN_OS_RELEASE

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

# Standard env every senku image carries. Previously inherited via
# `base = //oci/distroless/static` (which set these on its distroless_matrix).
# Composition-style images set this explicitly. `SSL_CERT_FILE` resolves on
# both Debian (native path) and Hummingbird (Debian-compat symlink shipped
# by Hummingbird's ca-certificates rpm).
DEFAULT_PATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

DEFAULT_ENV = {
    "PATH": DEFAULT_PATH,
    "SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
}

# Debug images append /busybox to PATH so the shell can resolve commands.
DEFAULT_DEBUG_ENV = {
    "PATH": DEFAULT_PATH + ":/busybox",
}

# Debian "no-DSA, Minor issue" CVEs that Debian Security has triaged but not
# yet backported a fix for. We track Debian unstable (sid) so most fixes flow
# in via lockfile bumps; this list is what's still pending upstream as of the
# current snapshot. Companion `_cve_test_stale_ignores` test fails when any
# entry vanishes from the scan, forcing us to delete it.
#
# Currently empty: the last entry, glibc's CVE-2026-5435, stopped matching
# with the 2026-08-14 grype DB — the same way CVE-2026-5450 and CVE-2026-5928
# left before it. The threading stays in place (every image that scans Debian
# packages passes this list to `ignore_cves`), so the next no-DSA finding is
# one line.
DEBIAN_WONTFIX_CVES = []

# OpenSSL (libssl3t64 / openssl-provider-legacy 3.6.3-1) — unfixed in sid, so
# it can't ride in on a lockfile bump like most Debian findings do. Kept out
# of DEBIAN_WONTFIX_CVES because openssl is absent from the static and
# registry images, which share that list; an entry there would never match
# their scans and would fail their `_cve_test_stale_ignores`. Apply only to
# images that link libssl: cc, bash, nginx, and the nginx-based frontend apps.
# `_cve_test_stale_ignores` fires when an entry stops matching, forcing it to
# be removed once Debian ships a fix.
OPENSSL_WONTFIX_CVES = [
    # OCSP-response memory leak reachable from a malicious TLS server, i.e.
    # DoS only (CWE-401). Debian tracks it open in sid and forky at 3.6.3-1
    # with urgency "not yet assigned"; only the older bookworm/trixie/bullseye
    # branches are resolved, so there is nothing to pull forward. Upstream
    # notes the affected path runs only when a client sets
    # X509_V_FLAG_OCSP_RESP_CHECK[_ALL], which is off by default.
    "CVE-2026-54876",
]

# Busybox: only present in `*_debug_*` variants via `static_debug_layers`.
# Apply via `distroless_matrix(debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro])`.
# Keyed by distro because the distros patch independently: Hummingbird
# shipped busybox 1:1.37.0-7.3.hum1 with the fix, Debian hasn't yet.
BUSYBOX_WONTFIX_CVES = {
    "debian": [
        # Debian busybox-static 1:1.37.0-10.1 — High, no fix shipped yet.
        # Busybox is the canonical debug-image toolbox (same choice as Google
        # distroless `:debug` and Chainguard `:latest-dev`); accept the CVE
        # tax until a patched build ships. `_cve_test_stale_ignores` will fail
        # when this stops matching, forcing the entry to be removed.
        "CVE-2026-29004",
    ],
    "hummingbird": [],
}
