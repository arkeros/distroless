"""`crane_tag` — apply tags to an already-pushed digest.

The second half of `mirror_push`. `_push` publishes by digest only; this
target names that digest with the human-facing tags, and CI runs it only after
every artifact on the digest has been verified (ADR 0014). Splitting push from
tag is what closes the window in which `latest` could resolve to an image whose
provenance was not yet attached.

The tag list is expanded at build time from the workspace status file, using
rules_img's `{{.STABLE_*}}` placeholder syntax so BUILD files read the same as
they did when `image_push` carried the tags. The expanded list is a build
output (`<name>.tags`), so it can be inspected and tested without a registry.
"""

_DOC = """Tag a remote digest with `crane tag`, once per entry of `tag_list`.

```starlark
crane_tag(
    name = "image_tag",
    image = ":image",  # anything with a `digest` output group
    repository = "ghcr.io/arkeros/distroless/bash",
    tag_list = ["latest", "{{.STABLE_MONOREPO_SHORT_VERSION}}"],
)
```

`bazel run :image_tag` issues `crane tag <repository>@<digest> <tag>` for each
tag. Crane reads registry credentials from the Docker config, as it does on
the command line. Set `CRANE` in the environment to substitute another binary
(the tests use this to record the commands).
"""

_attrs = {
    "image": attr.label(
        mandatory = True,
        doc = "Label exposing a `digest` output group (rules_img's `image_manifest` / `image_index`).",
    ),
    "repository": attr.string(
        mandatory = True,
        doc = "Registry + repository path the digest was pushed to, e.g. `ghcr.io/arkeros/distroless/bash`. Must NOT contain a tag or digest.",
    ),
    "tag_list": attr.string_list(
        mandatory = True,
        allow_empty = False,
        doc = "Tags to apply. `{{.STABLE_KEY}}` placeholders are expanded from the workspace status file.",
    ),
    "workspace_status": attr.label(
        allow_single_file = True,
        doc = "Status file to expand placeholders from. Defaults to Bazel's stable-status.txt; tests pass a fixed file.",
    ),
    "_crane": attr.label(
        default = "@com_github_google_go_containerregistry_cmd_krane//:krane",
        executable = True,
        cfg = "exec",
    ),
    "_tpl": attr.label(
        default = "//oci:crane_tag.sh.tpl",
        allow_single_file = True,
    ),
    "_runfiles_lib": attr.label(
        default = "@bazel_tools//tools/bash/runfiles",
    ),
}

def _rlocation_path(ctx, file):
    """The path `rlocation` resolves, for a file in this or an external repo."""
    if file.short_path.startswith("../"):
        return file.short_path[len("../"):]
    return ctx.workspace_name + "/" + file.short_path

def _crane_tag_impl(ctx):
    repository = ctx.attr.repository
    if ":" in repository or "@" in repository:
        fail("`repository` must not contain a tag or digest, got: {}".format(repository))

    digest_files = ctx.attr.image[OutputGroupInfo].digest.to_list()
    if len(digest_files) != 1:
        fail("Expected exactly 1 file in `digest` output group of {}, got {}".format(
            ctx.attr.image.label,
            len(digest_files),
        ))
    digest_file = digest_files[0]

    status_file = ctx.file.workspace_status if ctx.attr.workspace_status else ctx.info_file

    # Expand `{{.KEY}}` from `KEY VALUE` lines of the status file. Any
    # placeholder left unexpanded is a typo or a key the status command does
    # not emit; either way a tag literally named `{{.X}}` must never reach a
    # registry, so fail the action instead.
    tags_template = ctx.actions.declare_file(ctx.label.name + ".tags.tpl")
    ctx.actions.write(tags_template, "\n".join(ctx.attr.tag_list) + "\n")
    ctx.actions.run_shell(
        inputs = [tags_template, status_file],
        outputs = [ctx.outputs.tags],
        command = """
set -euo pipefail
TEMPLATE="$1"; STATUS="$2"; OUTPUT="$3"
cp "${TEMPLATE}" "${OUTPUT}.work"
while IFS=' ' read -r KEY VALUE; do
  [[ -n "${KEY}" ]] || continue
  # `#` delimiter: values are versions and commits, never contain it.
  sed "s#{{\\.${KEY}}}#${VALUE}#g" "${OUTPUT}.work" > "${OUTPUT}.work.tmp"
  mv "${OUTPUT}.work.tmp" "${OUTPUT}.work"
done < "${STATUS}"
if grep -n '{{' "${OUTPUT}.work"; then
  echo "ERROR: unexpanded placeholder in tag list (see above); the workspace status file has no such key." >&2
  exit 1
fi
mv "${OUTPUT}.work" "${OUTPUT}"
""",
        arguments = [tags_template.path, status_file.path, ctx.outputs.tags.path],
        mnemonic = "CraneTagList",
        progress_message = "Expanding tag list for %{label}",
    )

    executable = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._tpl,
        output = executable,
        is_executable = True,
        substitutions = {
            "{{crane_rlocation}}": _rlocation_path(ctx, ctx.executable._crane),
            "{{digest_rlocation}}": _rlocation_path(ctx, digest_file),
            "{{repository}}": repository,
            "{{tags_rlocation}}": _rlocation_path(ctx, ctx.outputs.tags),
        },
    )

    runfiles = ctx.runfiles(
        files = [digest_file, ctx.outputs.tags, ctx.executable._crane],
        transitive_files = ctx.attr._runfiles_lib[DefaultInfo].files,
    )
    runfiles = runfiles.merge(ctx.attr._crane[DefaultInfo].default_runfiles)
    runfiles = runfiles.merge(ctx.attr._runfiles_lib[DefaultInfo].default_runfiles)

    return [
        DefaultInfo(
            files = depset([executable]),
            executable = executable,
            runfiles = runfiles,
        ),
        OutputGroupInfo(tags = depset([ctx.outputs.tags])),
    ]

crane_tag = rule(
    implementation = _crane_tag_impl,
    attrs = _attrs,
    outputs = {"tags": "%{name}.tags"},
    doc = _DOC,
    executable = True,
)
