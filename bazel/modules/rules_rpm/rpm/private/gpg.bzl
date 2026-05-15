"""GPG verification helpers used at lock time (repomd.xml.asc) and build time (per-rpm).

repository_rule context only — these wrap `rctx.execute` against the pin/extract
Go binaries, which embed openpgp via golang.org/x/crypto.

Two layers must be subverted simultaneously to swap a package:
  1. The repo's `repomd.xml.asc` detached signature over `repomd.xml`
     (verified at lock time by the pin tool).
  2. Each rpm carries its own signature header verified by `rpm-extract`
     against the same trust root.

The per-rpm SHA256 in the lockfile is the third backstop and the only one
that doesn't require an online check.
"""

def verify_detached(rctx, gpg_tool, signature, data, keyring):
    """Returns (ok: bool, stderr: str). Used by the pin repo rule."""
    res = rctx.execute([
        gpg_tool,
        "--verify-detached",
        "--keyring", keyring,
        "--signature", signature,
        "--data", data,
    ])
    return (res.return_code == 0, res.stderr)
