"common variables"

def quote(str):
    return '''"{}"'''.format(str)

OS_RELEASE = dict(
    PRETTY_NAME = "Distroless",
    NAME = "Debian GNU/Linux",
    ID = "debian",
    VERSION_ID = "{VERSION}",
    VERSION = "Debian GNU/Linux {VERSION} ({CODENAME})",
    HOME_URL = "https://github.com/arkeros/senku",
    SUPPORT_URL = "https://github.com/arkeros/senku/blob/main/oci/distroless/README.md",
    BUG_REPORT_URL = "https://github.com/arkeros/senku/issues/new",
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
# Apply via `distroless_matrix(debug_ignore_cves = ...)`.
BUSYBOX_WONTFIX_CVES = [
    # busybox-static 1:1.37.0-10.1 — High, no fix in sid yet. Busybox is
    # the canonical debug-image toolbox (same choice as Google distroless
    # `:debug` and Chainguard `:latest-dev`); accept the CVE tax until
    # Debian ships a patched build. `_cve_test_stale_ignores` will fail
    # when this stops matching, forcing the entry to be removed.
    "CVE-2026-29004",
]

# rust-coreutils (uutils) — ships in bash images for env/ls/cat (no
# libsystemd0 dep, unlike GNU coreutils). Surfaced by the
# `dpkg-matcher`-via-SBOM fix (`oci/supply_chain.bzl` strips the
# `debian/` prefix so source==name packages get tracker matches); was
# silently hidden before. TODO: triage — VEX with
# vulnerable_code_not_in_execute_path if these utils' affected
# subcommands aren't reachable from our images, bump the lockfile if
# upstream rust-coreutils 0.9+ fixes them, or remove the package and
# fall back to a GNU-coreutils slice. `_cve_test_stale_ignores` will
# fail the moment any of these stop matching, forcing the entry out.
RUST_COREUTILS_PENDING_TRIAGE_CVES = [
    "CVE-2026-35341",
    "CVE-2026-35352",
    "CVE-2026-35368",
]
