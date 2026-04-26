"""Per-tf_root artifacts derived from declared providers.

`_tf_root_provider_artifacts` consumes the `providers` deps from a
`tf_root` and emits, all under the same `<tf_root_name>/` subdir of
the calling package:

- `.terraform.lock.hcl` — multi-platform lockfile that pins every
  declared provider with its `h1:` hashes.
- `providers.tf.json`   — `terraform { required_providers { … } }`
  block, kept in its own file so `tf_root`'s `main.tf.json` writer
  doesn't need to know about provider deps.
- `_providers/registry.terraform.io/<src>/<ver>/{index.json,
  <ver>.json, terraform-provider-…zip}` for every declared
  provider — symlinks + index JSONs in the layout `terraform
  providers mirror` produces.

`.terraformrc` is **NOT** emitted here. `filesystem_mirror.path`
requires an absolute path that's only knowable at run time (the
bazel-bin path is per-host); the runner script writes a fresh
`.terraformrc` into the workdir at terraform-launch time. This keeps
the bazel artifacts rule cleanly hermetic — same output across
hosts — and avoids the placeholder/substitution dance an
absolute-path-baked-in `.terraformrc` would require.
"""

load(":provider.bzl", "PLATFORMS", "TerraformProviderInfo")

def _short_name(source):
    """`hashicorp/google` → `google`. Used as both the
    `required_providers` key and the lockfile-section identifier."""
    return source.split("/")[-1]

_LOCKFILE_HEADER = """# This file is maintained automatically by "terraform init".
# Manual edits may be lost in future updates.

"""

def _render_lockfile(infos):
    """Render the document terraform itself would write after a fresh
    init against our filesystem mirror. Matching its output byte-for-byte
    keeps `terraform init` from rewriting the bazel output, which would
    cause cache churn on every plan."""
    blocks = []
    for info in infos:
        for platform in PLATFORMS:
            if platform not in info.hashes:
                fail("provider {} missing hash for {}".format(info.source, platform))
        # Terraform sorts the hashes alphabetically (by full `h1:…`
        # string) on write; mimic that.
        sorted_hashes = sorted([info.hashes[p] for p in PLATFORMS])
        hash_lines = "\n".join(['    "%s",' % h for h in sorted_hashes])
        blocks.append(
            'provider "registry.terraform.io/{source}" {{\n'.format(source = info.source) +
            '  version     = "{version}"\n'.format(version = info.version) +
            '  constraints = "{version}"\n'.format(version = info.version) +
            "  hashes = [\n" +
            hash_lines + "\n" +
            "  ]\n" +
            "}\n",
        )
    return _LOCKFILE_HEADER + "\n".join(blocks)

def _render_required_providers(infos):
    """Emit `terraform { required_providers { … } }` as a single
    nested-object JSON document. (`required_providers` is a meta-block,
    not a block-list; terraform expects a single object, not an array
    of objects, even in JSON syntax.)"""
    required = {}
    for info in infos:
        required[_short_name(info.source)] = {
            "source": info.source,
            "version": info.version,
        }
    return json.encode_indent(
        {"terraform": {"required_providers": required}},
        indent = "  ",
    ) + "\n"

def _tf_root_provider_artifacts_impl(ctx):
    infos = [dep[TerraformProviderInfo] for dep in ctx.attr.providers]
    gen_dir = ctx.attr.gen_dir

    outputs = []

    # 1. .terraform.lock.hcl
    lockfile = ctx.actions.declare_file(gen_dir + "/.terraform.lock.hcl")
    ctx.actions.write(output = lockfile, content = _render_lockfile(infos))
    outputs.append(lockfile)

    # 2. providers.tf.json — kept separate from main.tf.json so the
    # tf_root macro doesn't need to know provider metadata at macro
    # time. Terraform auto-loads every *.tf.json in the workdir.
    providers_json = ctx.actions.declare_file(gen_dir + "/providers.tf.json")
    ctx.actions.write(output = providers_json, content = _render_required_providers(infos))
    outputs.append(providers_json)

    # 3. Mirror tree (packed). Same layout `terraform providers mirror`
    # produces — every file (zips + index JSONs) sits flat under
    # `<host>/<ns>/<type>/`, no per-version subdirectory:
    #
    #   <host>/<ns>/<type>/index.json        — known versions
    #   <host>/<ns>/<type>/<version>.json    — archive map per platform
    #   <host>/<ns>/<type>/terraform-provider-<type>_<version>_<os>_<arch>.zip
    for info in infos:
        namespace, ptype = info.source.split("/")
        prefix = "{gen_dir}/_providers/registry.terraform.io/{ns}/{ptype}".format(
            gen_dir = gen_dir,
            ns = namespace,
            ptype = ptype,
        )

        # Per-platform archive symlinks + per-archive entries for the
        # version index. The url is relative to the version index file
        # itself — i.e. just the zip basename.
        archives_index = {}
        for platform in PLATFORMS:
            if platform not in info.archives:
                fail("provider {} missing archive for {}".format(info.source, platform))
            files = info.archives[platform].to_list()
            if len(files) != 1:
                fail("provider {} archive for {} should be exactly one zip; got {}".format(info.source, platform, len(files)))
            zip_file = files[0]
            out = ctx.actions.declare_file("{}/{}".format(prefix, zip_file.basename))
            ctx.actions.symlink(output = out, target_file = zip_file)
            outputs.append(out)
            archives_index[platform] = {
                "url": zip_file.basename,
                "hashes": [info.hashes[platform]],
            }

        index_json = ctx.actions.declare_file(prefix + "/index.json")
        ctx.actions.write(
            output = index_json,
            content = json.encode_indent(
                {"versions": {info.version: {}}},
                indent = "  ",
            ),
        )
        outputs.append(index_json)

        version_json = ctx.actions.declare_file("{}/{}.json".format(prefix, info.version))
        ctx.actions.write(
            output = version_json,
            content = json.encode_indent(
                {"archives": archives_index},
                indent = "  ",
            ),
        )
        outputs.append(version_json)

    return [DefaultInfo(files = depset(outputs))]

tf_root_provider_artifacts = rule(
    implementation = _tf_root_provider_artifacts_impl,
    attrs = {
        "providers": attr.label_list(
            providers = [TerraformProviderInfo],
            doc = "Provider deps; typically labels into `@terraform_providers`.",
        ),
        "gen_dir": attr.string(
            mandatory = True,
            doc = "Subdirectory under the calling package where outputs land. Must match the `tf_root` `name` so artifacts share a directory with `main.tf.json` / `backend.tf.json`.",
        ),
    },
)
