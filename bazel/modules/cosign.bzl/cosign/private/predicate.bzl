"""Implementation of `slsa_predicate`.

Produces a SLSA v1.0 ProvenanceStatement *predicate* (the body of an in-toto
attestation, without the wrapper Statement — cosign attaches that at attest
time, with the image's digest as the Statement's subject).

Phase 1 (current): minimal `buildDefinition` + `runDetails.builder.id`.
Phase 2 (future): add `resolvedDependencies` from the same `gather_metadata`
aspect that drives SBOM generation.

Workspace status keys (`STABLE_*` from `bazel/workspace_status.sh`) can be
referenced as `{{STABLE_KEY_NAME}}` placeholders inside any string attribute
value; substitution happens at action time.
"""

_DOC = """Build a SLSA v1.0 provenance predicate JSON file.

```starlark
load("@cosign.bzl//cosign:defs.bzl", "slsa_predicate")

slsa_predicate(
    name = "image_predicate",
    build_type = "https://github.com/arkeros/senku/cosign.bzl/v1/bazel-mirror",
    builder_id = "https://github.com/arkeros/senku/.github/workflows/ci.yaml@refs/heads/main",
    external_parameters = {
        "bazelTarget": "//oci/distroless/static:image_mirror",
        "sourceUri": "git+https://github.com/arkeros/senku@{{STABLE_GIT_COMMIT}}",
    },
    internal_parameters = {
        "monorepoVersion": "{{STABLE_MONOREPO_VERSION}}",
    },
)
```

Pair with `cosign_attest(type = "slsaprovenance", predicate = ":image_predicate")`.
"""

_attrs = {
    "build_type": attr.string(
        mandatory = True,
        doc = "URI identifying the build type. Per SLSA v1.0, should be a stable, dereferenceable URI describing the schema of `externalParameters` and `internalParameters`.",
    ),
    "builder_id": attr.string(
        mandatory = True,
        doc = "URI identifying the builder. For GitHub Actions: the workflow URL, e.g. `https://github.com/<org>/<repo>/.github/workflows/<file>.yml@refs/heads/main`.",
    ),
    "external_parameters": attr.string_dict(
        doc = "External (caller-supplied) parameters that influenced the build. Values may use `{{STABLE_KEY}}` placeholders.",
    ),
    "internal_parameters": attr.string_dict(
        doc = "Internal (build-system-derived) parameters. Values may use `{{STABLE_KEY}}` placeholders.",
    ),
}

def _slsa_predicate_impl(ctx):
    template_file = ctx.actions.declare_file(ctx.label.name + ".tpl.json")
    output_file = ctx.actions.declare_file(ctx.label.name + ".json")

    predicate = {
        "buildDefinition": {
            "buildType": ctx.attr.build_type,
            "externalParameters": dict(ctx.attr.external_parameters),
            "internalParameters": dict(ctx.attr.internal_parameters),
        },
        "runDetails": {
            "builder": {
                "id": ctx.attr.builder_id,
            },
        },
    }
    ctx.actions.write(template_file, json.encode_indent(predicate))

    # Substitute {{STABLE_KEY}} placeholders at action time using values from
    # the workspace status file. STABLE_* keys live in ctx.info_file.
    ctx.actions.run_shell(
        inputs = [template_file, ctx.info_file],
        outputs = [output_file],
        command = """
set -euo pipefail
TEMPLATE="$1"
INFO_FILE="$2"
OUTPUT="$3"
cp "${TEMPLATE}" "${OUTPUT}.work"
while IFS=' ' read -r KEY VALUE; do
  case "${KEY}" in
    STABLE_*)
      # `#` delimiter avoids escaping `/` in URI values.
      sed "s#{{${KEY}}}#${VALUE}#g" "${OUTPUT}.work" > "${OUTPUT}.work.tmp"
      mv "${OUTPUT}.work.tmp" "${OUTPUT}.work"
      ;;
  esac
done < "${INFO_FILE}"
mv "${OUTPUT}.work" "${OUTPUT}"
""",
        arguments = [template_file.path, ctx.info_file.path, output_file.path],
        mnemonic = "SlsaPredicate",
        progress_message = "Generating SLSA predicate %{label}",
    )

    return DefaultInfo(files = depset([output_file]))

slsa_predicate = rule(
    implementation = _slsa_predicate_impl,
    attrs = _attrs,
    doc = _DOC,
)
