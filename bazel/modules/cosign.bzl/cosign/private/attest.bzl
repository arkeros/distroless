"""Implementation of `cosign_attest`."""

_DOC = """Attach an in-toto attestation (e.g. SLSA provenance, SBOM) to an OCI
image at a remote registry.

```starlark
load("@rules_img//img:image.bzl", "image_index")
load("@cosign.bzl//cosign:defs.bzl", "cosign_attest", "slsa_predicate")

image_index(name = "image", ...)

slsa_predicate(
    name = "image_predicate",
    image = ":image",
    ...
)

cosign_attest(
    name = "image_attest_provenance",
    image = ":image",
    repository = "ghcr.io/arkeros/senku/distroless/static",
    type = "slsaprovenance",
    predicate = ":image_predicate",
)
```

Run with `bazel run :image_attest_provenance`. Same key-mode rules as
`cosign_sign`: keyless by default, KMS / file-key via `COSIGN_KEY`.
"""

_VALID_TYPES = ["slsaprovenance", "slsaprovenance02", "slsaprovenance1", "spdx", "spdxjson", "cyclonedx", "link", "vuln", "openvex", "custom"]

_attrs = {
    "image": attr.label(
        mandatory = True,
        doc = "Label of the image to attest. Must expose a `digest` output group.",
    ),
    "repository": attr.string(
        doc = (
            "Registry + repository path the attestation will be pushed to, e.g. " +
            "`ghcr.io/arkeros/senku/distroless/static`. Must NOT contain a tag or digest."
        ),
    ),
    "type": attr.string(
        mandatory = True,
        values = _VALID_TYPES,
        doc = "Attestation predicate type. Passed to `cosign attest --type=<type>`.",
    ),
    "predicate": attr.label(
        mandatory = True,
        allow_single_file = True,
        doc = "Label of the predicate file (e.g. SLSA provenance JSON, CycloneDX SBOM).",
    ),
    "referrers_mode": attr.string(
        values = ["", "legacy", "oci-1-1"],
        default = "",
        doc = (
            "Pass `--registry-referrers-mode=<value>` to cosign. " +
            "`oci-1-1` forces the OCI 1.1 referrers API (subject field) on " +
            "the legacy non-bundle code path. Requires registry support " +
            "(ghcr.io, ECR, GAR, Harbor >=2.8, distribution >=2.8); also " +
            "requires `COSIGN_EXPERIMENTAL=1`, which the wrapper auto-sets " +
            "when this attr is `oci-1-1` so callers don't need to. " +
            "**Most callers don't need this attr.** Cosign 3.x defaults " +
            "`--new-bundle-format=true`, and the bundle path writes via " +
            "OCI 1.1 referrers unconditionally — `--registry-referrers-mode` " +
            "only governs the legacy non-bundle path. Set this attr only " +
            "when `--new-bundle-format=false` is being passed at runtime, " +
            "or as defensive depth against a future cosign default flip."
        ),
    ),
    "_attest_sh_tpl": attr.label(
        default = "//cosign/private:attest.sh.tpl",
        allow_single_file = True,
    ),
}

def _validate_repository(repository):
    if repository.find(":") != -1 or repository.find("@") != -1:
        fail("`repository` must not contain a tag or digest, got: {}".format(repository))

def _cosign_attest_impl(ctx):
    if ctx.attr.repository:
        _validate_repository(ctx.attr.repository)

    cosign = ctx.toolchains["@cosign.bzl//cosign:toolchain"]
    digest_files = ctx.attr.image[OutputGroupInfo].digest.to_list()
    if len(digest_files) != 1:
        fail("Expected exactly 1 file in `digest` output group of {}, got {}".format(
            ctx.attr.image.label,
            len(digest_files),
        ))
    digest_file = digest_files[0]

    fixed_args = [
        "--type",
        ctx.attr.type,
        "--predicate",
        ctx.file.predicate.short_path,
    ]
    if ctx.attr.repository:
        fixed_args.extend(["--repository", ctx.attr.repository])
    if ctx.attr.referrers_mode:
        fixed_args.extend(["--registry-referrers-mode", ctx.attr.referrers_mode])

    executable = ctx.actions.declare_file("cosign_attest_{}.sh".format(ctx.label.name))
    ctx.actions.expand_template(
        template = ctx.file._attest_sh_tpl,
        output = executable,
        is_executable = True,
        substitutions = {
            "{{cosign_path}}": cosign.cosign_info.cosign_binary.short_path,
            "{{digest_file}}": digest_file.short_path,
            "{{fixed_args}}": " ".join([repr(a) for a in fixed_args]),
        },
    )

    runfiles = ctx.runfiles(files = [digest_file, ctx.file.predicate])
    runfiles = runfiles.merge(cosign.default.default_runfiles)

    return DefaultInfo(executable = executable, runfiles = runfiles)

cosign_attest = rule(
    implementation = _cosign_attest_impl,
    attrs = _attrs,
    doc = _DOC,
    executable = True,
    toolchains = ["@cosign.bzl//cosign:toolchain"],
)
