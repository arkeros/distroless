"""Public API for cosign.bzl.

Two rules sit on top of a `cosign_toolchain`:

  * `cosign_sign` — `cosign sign --recursive` against a rules_img image's digest.
  * `cosign_attest` — `cosign attest --type=<predicate-type> --predicate=<file>`.

There is deliberately no rule that *writes* provenance: a predicate the build
produces about itself is forgeable by the build (SLSA Build L2 at best). The
platform attaches provenance; see distroless ADR 0014.

Both rules read the signed image's digest from the `digest` output group
exposed by rules_img's `image_manifest` / `image_index`. No `index.json`
parsing, no stdout scraping.

Key mode is runtime-configurable: setting `COSIGN_KEY` in the environment
switches sign/attest from keyless OIDC (Fulcio + Rekor) to a key reference
(KMS, file). Default is keyless.
"""

load("//cosign/private:attest.bzl", _cosign_attest = "cosign_attest")
load("//cosign/private:sign.bzl", _cosign_sign = "cosign_sign")

cosign_sign = _cosign_sign
cosign_attest = _cosign_attest
