"""Per-tf_root artifacts derived from declared providers.

`_tf_root_provider_artifacts` consumes the `providers` deps from a
`tf_root` and emits, all under the same `<tf_root_name>/` subdir of
the calling package:

- `.terraform.lock.hcl` — multi-platform lockfile that pins every
  declared provider with its `h1:` hashes.
- `.terraformrc`        — CLI config with a `filesystem_mirror`
  block pointing at the sibling `_providers/` tree. Contains an
  `@@MIRROR_PATH@@` placeholder that the runner script substitutes
  with the absolute cwd at terraform-launch time (filesystem_mirror
  requires absolute paths).
- `providers.tf.json`   — `terraform { required_providers { … } }`
  block, kept in its own file so `tf_root`'s `main.tf.json` writer
  doesn't need to know about provider deps.
- `_providers/registry.terraform.io/<src>/<ver>/<os_arch>/<binary>`
  for every provider × platform — symlinks pointing at the
  `_provider_archive_repo` files materialized by the toolchain
  module extension.

Together these files turn the bazel-bin output dir into a
self-contained terraform working directory: `cd
bazel-bin/<package>/<name>/ && terraform init` works offline (modulo
the placeholder substitution; see `run.sh`).
"""

load(":provider.bzl", "PLATFORMS", "TerraformProviderInfo")

def _short_name(source):
    """`hashicorp/google` → `google`. Used as both the
    `required_providers` key and the lockfile-section identifier."""
    return source.split("/")[-1]

def _render_lockfile(infos):
    blocks = []
    for info in infos:
        for platform in PLATFORMS:
            if platform not in info.hashes:
                fail("provider {} missing hash for {}".format(info.source, platform))
        hash_lines = ",\n    ".join(['"%s"' % info.hashes[p] for p in PLATFORMS])
        blocks.append(
            'provider "registry.terraform.io/{source}" {{\n'.format(source = info.source) +
            '  version     = "{version}"\n'.format(version = info.version) +
            '  constraints = "{version}"\n'.format(version = info.version) +
            "  hashes = [\n" +
            "    " + hash_lines + ",\n" +
            "  ]\n" +
            "}\n",
        )
    return "\n".join(blocks)

# `@@MIRROR_PATH@@` is substituted by `run.sh` (and by the manual
# `setup-terraformrc.sh` helper) with the absolute path of the bazel-bin
# dir hosting `_providers/`. Filesystem-mirror paths in terraform must
# be absolute — the only thing not knowable at Bazel build time.
_TERRAFORMRC = """provider_installation {
  filesystem_mirror {
    path    = "@@MIRROR_PATH@@/_providers"
    include = ["registry.terraform.io/*/*"]
  }
  direct {
    exclude = ["registry.terraform.io/*/*"]
  }
}
"""

def _render_required_providers(infos):
    """Emit the `terraform { required_providers { ... } }` JSON block."""
    required = {}
    for info in infos:
        required[_short_name(info.source)] = {
            "source": info.source,
            "version": info.version,
        }
    return json.encode_indent(
        {"terraform": [{"required_providers": [required]}]},
        indent = "  ",
    ) + "\n"

# Terraform's filesystem-mirror layout is rigid:
#   <mirror>/<hostname>/<namespace>/<type>/<version>/<os_arch>/<binary>
# We pin <hostname> to registry.terraform.io (the only registry we use)
# and split <source> into <namespace>/<type>.
def _mirror_path(gen_dir, info, platform, basename):
    namespace, ptype = info.source.split("/")
    return "{gen_dir}/_providers/registry.terraform.io/{namespace}/{ptype}/{version}/{platform}/{basename}".format(
        gen_dir = gen_dir,
        namespace = namespace,
        ptype = ptype,
        version = info.version,
        platform = platform,
        basename = basename,
    )

def _tf_root_provider_artifacts_impl(ctx):
    infos = [dep[TerraformProviderInfo] for dep in ctx.attr.providers]
    gen_dir = ctx.attr.gen_dir

    outputs = []

    # 1. .terraform.lock.hcl
    lockfile = ctx.actions.declare_file(gen_dir + "/.terraform.lock.hcl")
    ctx.actions.write(output = lockfile, content = _render_lockfile(infos))
    outputs.append(lockfile)

    # 2. .terraformrc
    terraformrc = ctx.actions.declare_file(gen_dir + "/.terraformrc")
    ctx.actions.write(output = terraformrc, content = _TERRAFORMRC)
    outputs.append(terraformrc)

    # 3. providers.tf.json — kept separate from main.tf.json so the
    # tf_root macro doesn't need to know provider metadata at macro
    # time. Terraform auto-loads every *.tf.json in the workdir.
    providers_json = ctx.actions.declare_file(gen_dir + "/providers.tf.json")
    ctx.actions.write(output = providers_json, content = _render_required_providers(infos))
    outputs.append(providers_json)

    # 4. Mirror tree.
    for info in infos:
        for platform in PLATFORMS:
            if platform not in info.archives:
                fail("provider {} missing archive for {}".format(info.source, platform))
            files = info.archives[platform].to_list()
            # Each provider zip contains exactly one binary at the root
            # named `terraform-provider-<type>_v<version>...`.
            for f in files:
                out = ctx.actions.declare_file(_mirror_path(gen_dir, info, platform, f.basename))
                ctx.actions.symlink(output = out, target_file = f)
                outputs.append(out)

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
