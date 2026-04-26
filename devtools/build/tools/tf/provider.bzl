"""Terraform provider as a first-class Bazel target.

`tf_provider_target` is the rule emitted (one per declared provider) by
the `@terraform_providers` hub repo's generated BUILD. Each instance
carries everything `tf_root` needs to build a hermetic per-root
working directory:

- `source` + `version`     for the `required_providers` block.
- `hashes`                  for the `.terraform.lock.hcl` file.
- `archives`                for the filesystem-mirror tree under
                            `_providers/registry.terraform.io/...`.

Callers don't instantiate this rule directly — they `load` and
reference the targets via labels like `@terraform_providers//:google`,
exactly the way `go_library(deps = ["@com_github_..."])` references
gazelle-managed module repos.
"""

# Platform identifiers used everywhere in this stack. Mirrors the keys
# in `bazel/include/terraform.providers.lock.bzl` and the suffix used
# by HashiCorp's release tarballs (`<name>_<version>_<os>_<arch>.zip`).
PLATFORMS = ["darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"]

TerraformProviderInfo = provider(
    doc = "One declared terraform provider plus its per-platform binaries and lockfile hashes.",
    fields = {
        "source": "Provider source address (e.g. `hashicorp/google`).",
        "version": "Pinned exact version (e.g. `7.29.0`).",
        "hashes": "Dict {platform: 'h1:...'} consumed by the .terraform.lock.hcl renderer.",
        "archives": "Dict {platform: depset of File} pointing at the unpacked provider binary for each platform.",
    },
)

def _tf_provider_target_impl(ctx):
    archives = {
        platform: target.files
        for platform, target in ctx.attr.archives.items()
    }
    missing = [p for p in PLATFORMS if p not in archives]
    if missing:
        fail("tf_provider_target {} is missing archives for platforms: {}".format(
            ctx.label,
            missing,
        ))
    return [TerraformProviderInfo(
        source = ctx.attr.source,
        version = ctx.attr.version,
        hashes = ctx.attr.hashes,
        archives = archives,
    )]

tf_provider_target = rule(
    implementation = _tf_provider_target_impl,
    attrs = {
        "source": attr.string(
            mandatory = True,
            doc = "Provider source, e.g. `hashicorp/google`.",
        ),
        "version": attr.string(
            mandatory = True,
            doc = "Exact version, e.g. `7.29.0`.",
        ),
        "hashes": attr.string_dict(
            mandatory = True,
            doc = "platform -> `h1:<base64-of-sha256>`. One entry per supported platform; the lockfile renderer fails loudly if any are missing.",
        ),
        "archives": attr.string_keyed_label_dict(
            mandatory = True,
            allow_files = True,
            doc = "platform -> filegroup label (one of the `_provider_archive_repo` outputs from the toolchain extension).",
        ),
    },
    doc = "Hub-repo target representing one declared terraform provider.",
)
