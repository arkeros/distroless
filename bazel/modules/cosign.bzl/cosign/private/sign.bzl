"""Implementation of `cosign_sign`."""

_DOC = """Sign an OCI image at a remote registry.

Reads the image's digest from the `digest` output group exposed by rules_img's
`image_manifest` / `image_index`. Signs `<repository>@<digest>` so signatures
bind to digests, not tags.

```starlark
load("@rules_img//img:image.bzl", "image_index")
load("@cosign.bzl//cosign:defs.bzl", "cosign_sign")

image_index(name = "image", ...)

cosign_sign(
    name = "image_sign",
    image = ":image",
    repository = "ghcr.io/arkeros/senku/distroless/static",
)
```

Run with `bazel run :image_sign`. Default mode is keyless (Fulcio + Rekor) —
the runner must have an OIDC token in env (`ACTIONS_ID_TOKEN_REQUEST_*`).
Setting `COSIGN_KEY` in the environment switches to a key reference (KMS
URI, file path); the rule itself is runner-agnostic.

`--repository` may be overridden at runtime: `bazel run :image_sign -- --repository=other.io/foo`.
"""

_attrs = {
    "image": attr.label(
        mandatory = True,
        doc = "Label of the image to sign. Must expose a `digest` output group (rules_img's `image_manifest` / `image_index`).",
    ),
    "repository": attr.string(
        doc = (
            "Registry + repository path the image will be signed at, e.g. " +
            "`ghcr.io/arkeros/senku/distroless/static`. Must NOT contain a tag or digest. " +
            "Can be overridden at runtime via `--repository`."
        ),
    ),
    "recursive": attr.bool(
        default = True,
        doc = "Pass `--recursive` to `cosign sign` so multi-arch indexes have each per-platform manifest signed too.",
    ),
    "_sign_sh_tpl": attr.label(
        default = "//cosign/private:sign.sh.tpl",
        allow_single_file = True,
    ),
}

def _validate_repository(repository):
    if repository.find(":") != -1 or repository.find("@") != -1:
        fail("`repository` must not contain a tag or digest, got: {}".format(repository))

def _cosign_sign_impl(ctx):
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

    fixed_args = []
    if ctx.attr.repository:
        fixed_args.extend(["--repository", ctx.attr.repository])
    if ctx.attr.recursive:
        fixed_args.append("--recursive")

    executable = ctx.actions.declare_file("cosign_sign_{}.sh".format(ctx.label.name))
    ctx.actions.expand_template(
        template = ctx.file._sign_sh_tpl,
        output = executable,
        is_executable = True,
        substitutions = {
            "{{cosign_path}}": cosign.cosign_info.cosign_binary.short_path,
            "{{digest_file}}": digest_file.short_path,
            "{{fixed_args}}": " ".join([repr(a) for a in fixed_args]),
        },
    )

    runfiles = ctx.runfiles(files = [digest_file])
    runfiles = runfiles.merge(cosign.default.default_runfiles)

    return DefaultInfo(executable = executable, runfiles = runfiles)

cosign_sign = rule(
    implementation = _cosign_sign_impl,
    attrs = _attrs,
    doc = _DOC,
    executable = True,
    toolchains = ["@cosign.bzl//cosign:toolchain"],
)
