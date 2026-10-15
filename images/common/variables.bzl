"common variables"

def quote(str):
    return '''"{}"'''.format(str)

DEBIAN_OS_RELEASE = dict(
    PRETTY_NAME = "Distroless",
    NAME = "Debian GNU/Linux",
    ID = "debian",
    VERSION_ID = "{VERSION}",
    VERSION = "Debian GNU/Linux {VERSION} ({CODENAME})",
    HOME_URL = "https://github.com/arkeros/distroless",
    SUPPORT_URL = "https://github.com/arkeros/distroless/blob/main/docs/images.md",
    BUG_REPORT_URL = "https://github.com/arkeros/distroless/issues/new",
)

# Hummingbird-derived images. `ID=hummingbird` is the scanner-routing key
# (grype/trivy match exact-string only, no ID_LIKE fallback — see ADR 0007);
# NAME / PRETTY_NAME carry the distroless brand for human readers. VERSION_ID is
# the Hummingbird snapshot revision (Unix timestamp from repomd.xml).
HUMMINGBIRD_OS_RELEASE = dict(
    PRETTY_NAME = "distroless.io (Hummingbird-derived)",
    NAME = "distroless.io",
    ID = "hummingbird",
    ID_LIKE = "rhel fedora",
    VERSION_ID = "{VERSION}",
    HOME_URL = "https://github.com/arkeros/distroless",
    SUPPORT_URL = "https://github.com/arkeros/distroless/blob/main/docs/images.md",
    BUG_REPORT_URL = "https://github.com/arkeros/distroless/issues/new",
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

# Standard env every distroless image carries. Previously inherited via
# `base = //images/static` (which set these on its distroless_matrix).
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

# OpenSSL (libssl3t64 / openssl-provider-legacy) CVEs that Debian Security has
# triaged but not yet fixed in sid, so they can't ride in on a lockfile bump
# like most Debian findings do. Kept out of DEBIAN_WONTFIX_CVES because openssl
# is absent from the static and registry images, which share that list; an
# entry there would never match their scans and would fail their
# `_cve_test_stale_ignores`. Apply only to images that link libssl: cc, bash,
# nginx, and the nginx-based frontend apps.
#
# Currently empty: the last entry, the OCSP-response memory leak
# CVE-2026-54876, stopped matching once sid shipped openssl 3.6.4-1. The
# threading stays in place (every image that links libssl passes this list to
# `ignore_cves`), so the next unfixed openssl finding is one line.
OPENSSL_WONTFIX_CVES = []

# gcc runtime (libstdc++6 / libgcc-s1 / gcc-16-base) CVEs Debian has fixed
# only in the gcc-13 and gcc-14 source packages (13.5.0-3, 14.4.0-3), while
# sid's libstdc++6 is built from gcc-16, which the tracker still lists as
# vulnerable at 16.2.0-3 — so, like the OpenSSL list above, nothing a
# lockfile bump can ride in on. Both are flaws in libstdc++ itself:
# CVE-2026-102010 is `erase_if` on a `priority_queue` leaving a dangling
# entry pointer (header-only template code, nothing of it is in the shared
# object), CVE-2026-95619 an integer overflow in aligned `operator new`.
# Neither bash nor cc ships a binary that imports aligned `operator new`
# (checked with `nm -D --undefined-only` over every ELF object in the
# images), but a python or nginx consumer may, so this is an ignore, not a
# `not_affected` VEX statement. Apply to every image built on cc — it is the
# cc layer that ships libstdc++ — and never to static, which has no C++
# runtime and would fail its `_cve_test_stale_ignores` on an entry that
# never matches. `_cve_test_stale_ignores` prunes these the day sid's
# gcc-16 carries the fix.
GCC_WONTFIX_CVES = [
    "CVE-2026-102010",
    "CVE-2026-95619",
]

# Busybox: only present in `*_debug_*` variants via `static_debug_layers`.
# Apply via `distroless_matrix(debug_ignore_cves = BUSYBOX_WONTFIX_CVES[distro])`.
# Keyed by distro because the distros patch independently: Hummingbird
# shipped busybox 1:1.37.0-7.3.hum1 with the fix, Debian hasn't yet.
BUSYBOX_WONTFIX_CVES = {
    "debian": [
        # Debian busybox-static 1:1.38.0-3+b1 — High, no fix shipped yet.
        # Busybox is the canonical debug-image toolbox (same choice as Google
        # distroless `:debug` and Chainguard `:latest-dev`); accept the CVE
        # tax until a patched build ships. `_cve_test_stale_ignores` will fail
        # when this stops matching, forcing the entry to be removed.
        "CVE-2026-29004",
        # Same package, the 2026-10 batch: a pre-authentication heap overflow
        # in the TLS client's Montgomery reduction (CVE-2026-88830) and an
        # unbounded strlen over a romfs volume id (CVE-2026-88832). Neither
        # has a Debian fix at 1:1.38.0-3+b1; the debug image's busybox is
        # a shell for a human at a terminal, not a TLS client or a mount
        # helper, but that is the same argument the entry above makes.
        "CVE-2026-88830",
        "CVE-2026-88832",
    ],
    "hummingbird": [
        # Hummingbird busybox 1:1.37.0-9.4.hum1 — the same TLS Montgomery
        # overflow (CVE-2026-88830) is the one of the batch Red Hat has not
        # shipped a build for; grype's Hummingbird secdb lists it with no
        # fixed version. The other 2026-10 busybox CVEs are fixed in
        # 1.37.0-9.1 to 9.4 and no longer match.
        "CVE-2026-88830",
    ],
}
