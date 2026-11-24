"""Reusable VEX statements for CVEs Debian has *not* fixed yet, where the
vulnerable code is demonstrably unreachable from the images that ship the
package. Sibling of debian_fixed_vex.bzl (grype lagging Debian) and of
variables.bzl's `*_WONTFIX_CVES` (bare ignores, no justification).

Prefer a statement here over a `*_WONTFIX_CVES` entry whenever the
argument can be made per binary — a VEX document is what consumers get
attested alongside the image, an ignore list is only a test suppression.

Lifecycle is the same as the other lists: `_cve_test_stale_vex` fires the
moment the scanner stops flagging the CVE (Debian ships the fix, or the
package drops out of the image), and the statement gets deleted.
"""

# zlib1g. Empty: the last statement, CVE-2026-85091 (heap overflow in
# gz_vacate() on the gzFile write API, which no binary in these images
# imports), stopped matching with the 2026-11-24 grype database. The
# threading stays in place, so the next unreachable zlib finding is one
# statement.
ZLIB_UNFIXED_VEX_STATEMENTS = []
