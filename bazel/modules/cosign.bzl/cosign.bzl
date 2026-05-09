"""Re-export to allow syntax sugar: load("@cosign.bzl", "cosign_sign")."""

load("//cosign:defs.bzl", _cosign_attest = "cosign_attest", _cosign_sign = "cosign_sign", _slsa_predicate = "slsa_predicate")
load("//cosign/toolchain:toolchain.bzl", _cosign_toolchain = "cosign_toolchain")

cosign_sign = _cosign_sign
cosign_attest = _cosign_attest
slsa_predicate = _slsa_predicate
cosign_toolchain = _cosign_toolchain
