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
# The 2026-11-10 and 2026-11-17 OpenSSL advisories, plus fixes upstream
# landed without one. sid ships openssl 3.6.5-1, and 3.6 is out of upstream
# support ("3.1, 3.2, 3.3, 3.4, and 3.6 are out of support and have not been
# analysed"), so no 3.6.x carrying these fixes will be released: fixed
# releases exist only for 4.1.1, 4.0.4, 3.5.10 and 3.0.24. Debian's way out
# is the libssl3t64 -> libssl4 transition (openssl 4.1 in experimental since
# 2026-11-15; release.debian.org #1140179, acked 2026-11-24 to start "soon").
# Pulling libssl4 from experimental would not help: nginx, python and the
# rest of the cc layer link libssl.so.3 until they are rebuilt in the
# transition. OpenSSL rates the 2026-11-17 batch Low; grype reports them
# High from the distro scores. Bump images/debian.yaml once the transition
# has rebuilt nginx and python against libssl4, and
# `_cve_test_stale_ignores` then forces every entry here out.
OPENSSL_WONTFIX_CVES = [
    "CVE-2026-107493",  # use-after-free in PEM_X509_INFO_read_bio_ex() on a caller-supplied stack
    "CVE-2026-107494",  # QUIC: unbounded RETIRE_CONNECTION_ID frame queueing (DoS)
    "CVE-2026-107496",  # CMS encryption to an EC certificate with a crafted group
    "CVE-2026-121482",  # CMS / PKCS#7 SignedData verification
    "CVE-2026-124709",  # QUIC server: unbounded memory for rejected streams (DoS)
    "CVE-2026-84786",  # DANE-enabled TLS client NULL dereference
    "CVE-2026-88823",  # AES-GCM-SIV with zero-length AAD
    "CVE-2026-95691",  # QUIC: retransmitted CRYPTO frame releases packet state
    "CVE-2026-95692",  # AES-GCM-SIV with a short IV
]

# Hummingbird's openssl-libs 3.5.9-0.5.hum1 — the supported 3.5 LTS line,
# with Red Hat's backports. Only the QUIC RETIRE_CONNECTION_ID queueing DoS
# (CVE-2026-107494) is unfixed there: Red Hat lists Hummingbird's openssl
# and openssl3 as Affected with no build yet. Its sibling CVE-2026-124709 is
# fixed in 0.5.hum1 and handled by a `fixed` VEX statement
# (OPENSSL_HUMMINGBIRD_FIXED_VEX_STATEMENTS), not an ignore. Apply to every
# Hummingbird image that ships openssl-libs: all but static.
HUMMINGBIRD_OPENSSL_WONTFIX_CVES = [
    "CVE-2026-107494",
]

# glibc CVEs Debian has not fixed in sid. Kept out of DEBIAN_WONTFIX_CVES for
# the same reason as the OpenSSL list: static and the registry images have no
# glibc, so an entry there fails their `_cve_test_stale_ignores`. Apply to
# every Debian image built on cc.
#
# CVE-2026-84243: gettext loads a message catalog from any path named in
# LANGUAGE ('/' or ".." entries) unless the process is AT_SECURE. Unfixed in
# glibc 2.43-7 (Debian bug #1152640, <no-dsa> "Minor issue" for trixie);
# Hummingbird's glibc 2.43-9.3.hum1 carries the fix, which is why only the
# Debian images match. Exploiting it needs control of both the environment
# of a process in the container and a file in its filesystem.
GLIBC_WONTFIX_CVES = [
    "CVE-2026-84243",
]

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
#
# Currently empty: both entries stopped matching with the 2026-11-24 grype
# database. The threading stays in place, so the next gcc runtime finding
# is one line.
GCC_WONTFIX_CVES = []

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
        # The 2026-11 batch, unfixed at 1:1.38.0-3+b2 (Debian bug #1154070,
        # <no-dsa> "Minor issue" for trixie): TFTP (CVE-2026-124737), httpd
        # subdirectory handling (CVE-2026-124739), the udhcpc6 DHCPv6 client
        # (CVE-2026-124741), FTP download resume (CVE-2026-124747), the
        # httpd reverse proxy (CVE-2026-88829) and dpkg-deb archive
        # extraction (CVE-2026-90954). None of those applets is something
        # the debug shell runs on its own; same argument as above.
        "CVE-2026-124737",
        "CVE-2026-124739",
        "CVE-2026-124741",
        "CVE-2026-124747",
        "CVE-2026-88829",
        "CVE-2026-90954",
    ],
    "hummingbird": [
        # Hummingbird busybox 1:1.38.0-1.hum1, the same 2026-11 batch minus
        # the udhcpc6 one (CVE-2026-124741, not reported here): no Hummingbird
        # build carries the fixes yet. Same reasoning as the Debian entries.
        # The TLS Montgomery overflow CVE-2026-88830 stopped matching with
        # the 2026-11-24 grype database.
        "CVE-2026-124737",
        "CVE-2026-124739",
        "CVE-2026-124747",
        "CVE-2026-88829",
        "CVE-2026-90954",
    ],
}
