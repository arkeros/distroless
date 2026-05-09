"""Public API for cosign.bzl.

Three rules sit on top of a `cosign_toolchain`:

  * `cosign_sign` — `cosign sign --recursive` against a rules_img image's digest.
  * `cosign_attest` — `cosign attest --type=<predicate-type> --predicate=<file>`.
  * `slsa_predicate` — Bazel-built SLSA v1.0 ProvenanceStatement JSON, suitable
    as the `predicate` input to `cosign_attest(type = "slsaprovenance")`.

All three rules read the signed image's digest from the `digest` output group
exposed by rules_img's `image_manifest` / `image_index`. No `index.json`
parsing, no stdout scraping.

Key mode is runtime-configurable: setting `COSIGN_KEY` in the environment
switches sign/attest from keyless OIDC (Fulcio + Rekor) to a key reference
(KMS, file). Default is keyless.
"""

load("//cosign/private:attest.bzl", _cosign_attest = "cosign_attest")
load("//cosign/private:predicate.bzl", _slsa_predicate = "slsa_predicate")
load("//cosign/private:sign.bzl", _cosign_sign = "cosign_sign")

cosign_sign = _cosign_sign
cosign_attest = _cosign_attest
slsa_predicate = _slsa_predicate
