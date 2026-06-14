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
DEBIAN_WONTFIX_CVES = [
    # glibc (libc6) — all currently unfixed in sid 2.42-15
    "CVE-2026-5435",
    "CVE-2026-5450",
    "CVE-2026-5928",
]

# Busybox: only present in `*_debug_*` variants via `static_debug_layers`.
# Apply via `distroless_matrix(debug_ignore_cves = ...)`.
BUSYBOX_WONTFIX_CVES = [
    # Debian busybox-static 1:1.37.0-10.1 and Hummingbird busybox
    # 1:1.37.0-7.2.hum1 — High, no fix shipped by either distro yet.
    # Busybox is the canonical debug-image toolbox (same choice as Google
    # distroless `:debug` and Chainguard `:latest-dev`); accept the CVE
    # tax until a patched build ships. `_cve_test_stale_ignores` will fail
    # when this stops matching, forcing the entry to be removed.
    "CVE-2026-29004",
]

# OpenSSL (libssl3t64 / openssl / openssl-provider-legacy 3.6.2-1) — a batch
# of High/Critical CVEs all unfixed in sid as of the snapshot date (grype
# reports no fixed version). Unlike the glibc entries above these live in a
# separate list because openssl is absent from the static and java images,
# which share DEBIAN_WONTFIX_CVES; mixing them in there would make those
# images' `_cve_test_stale_ignores` fail. Apply only to images that link
# libssl (cc, bash, nginx, workstation). `_cve_test_stale_ignores` fires when
# any entry stops matching, forcing it to be removed once Debian ships a fix.
OPENSSL_WONTFIX_CVES = [
    "CVE-2026-7383",
    "CVE-2026-9076",
    "CVE-2026-34180",
    "CVE-2026-34181",
    "CVE-2026-34182",
    "CVE-2026-34183",
    "CVE-2026-42764",
    "CVE-2026-42765",
    "CVE-2026-45445",
    "CVE-2026-45447",
]
