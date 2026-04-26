# `tf_root` — Starlark-generated Terraform roots

Generates `main.tf.json` + `backend.tf.json` + `providers.tf.json` +
`.terraform.lock.hcl` + `.terraformrc` + a filesystem-mirror tree of
provider zips for one Terraform root, plus `:<name>.{plan,apply,destroy}`
runnable targets that exec terraform against the generated dir.

The output is a self-contained terraform working directory:
`bazel-bin/<package>/<name>/`. `bazel run :<name>.plan` runs terraform
in that dir; `terraform init` resolves providers from the local
mirror, never from `registry.terraform.io`.

For the broader design rationale (why Starlark-generated `.tf.json`
instead of HCL, why Aspect CLI for cross-root orchestration), see
[`/docs/infra-as-starlark.md`](../../../../docs/infra-as-starlark.md).

## Defining a root

```python
# infra/cloud/gcp/myroot/BUILD
load("//devtools/build/tools/tf:defs.bzl", "tf_root")
load("//devtools/build/tools/tf/resources:gcp.bzl", "google_provider")
load(":defs.bzl", "MY_DOCS", "PROJECT")

tf_root(
    name = "terraform",
    docs = [google_provider(project = PROJECT)] + MY_DOCS,
    providers = ["@terraform_providers//:google"],
    visibility = ["//visibility:public"],
)
```

`docs` is a list of resource structs (from `resource(...)` /
`remote_state(...)` / `output(...)`) and/or raw Terraform-shaped
dicts. The macro merges them into `main.tf.json`.

`providers` is a list of label deps into the `@terraform_providers`
hub. Each entry pulls the provider's per-platform sha256-pinned
archives into the per-root mirror tree and contributes its `h1:`
hashes to `.terraform.lock.hcl`. Skip the arg for a backend-only root
(no resources, no providers).

## Adding a new provider

Provider declarations live in
[`/bazel/include/terraform.MODULE.bazel`](../../../../bazel/include/terraform.MODULE.bazel)
alongside the toolchain pin. Add a new tag:

```python
terraform.provider(
    source = "hashicorp/random",
    version = "3.6.0",
)
```

Then regenerate the per-platform hash pins:

```
bazel run //devtools/build/tools/tf/providers/repin
git diff bazel/include/terraform.providers.lock.bzl
git commit
```

The pin tool fetches HashiCorp's `SHA256SUMS` for each declared
version, downloads each platform's zip, computes terraform's `h1:`
directory hash via `golang.org/x/mod/sumdb/dirhash`, and writes the
new `PROVIDER_HASHES` dict. Idempotent on a correctly-pinned spec —
re-running with no changes produces an empty diff.

After committing, the new provider is referenceable as
`@terraform_providers//:random` from any `tf_root(providers = […])`.

## Bumping a provider version

Same flow. Edit `version` in `terraform.MODULE.bazel`, run `:repin`,
review the diff, commit. Existing tf_roots pick up the new version
automatically (the lockfile + mirror are regenerated on next
`bazel build`).

## Manual `terraform` invocation

The bazel-bin output dir IS the terraform working directory. To run
terraform commands directly (without going through the
`:<name>.plan/apply/destroy` wrappers):

```
bazel build //infra/cloud/gcp/lb:terraform
cd bazel-bin/infra/cloud/gcp/lb/terraform/

# .terraformrc ships with an @@MIRROR_PATH@@ placeholder the wrapper
# normally substitutes at run time. For manual use, do it once:
sed "s|@@MIRROR_PATH@@|$(pwd)|g" .terraformrc > .terraformrc.runtime
export TF_CLI_CONFIG_FILE=$(pwd)/.terraformrc.runtime

terraform init    # resolves providers from ./_providers, no network
terraform plan
```

`terraform init` writes its own `.terraform/` mutable cache into the
bazel-bin dir. It survives until `bazel clean`. State proper lives in
the GCS backend, so a clean only forces a re-init, not a re-apply.

## What lands in `bazel-bin/<package>/<name>/`

| File | Source | Purpose |
|---|---|---|
| `main.tf.json`        | `tf_root.docs`              | Resources, outputs, data sources |
| `backend.tf.json`     | `tf_root.backend_*`         | `terraform { backend "gcs" { … } }` |
| `providers.tf.json`   | `tf_root.providers`         | `terraform { required_providers { … } }` |
| `.terraform.lock.hcl` | `tf_root.providers`         | Multi-platform `h1:` hashes |
| `.terraformrc`        | `tf_root.providers`         | `provider_installation { filesystem_mirror { … } direct { exclude = [*] } }` |
| `_providers/…`        | `tf_root.providers`         | Provider zip archives + index JSONs in the layout `terraform providers mirror` produces |

Bazel-managed; regenerated on every `bazel build` if any input
changes. Terraform's own outputs (`.terraform/`, `.terraformrc.runtime`,
`tfplan.bin`) live in the same dir but are not bazel-tracked.

## How `init` stays offline

The `.terraformrc` instructs terraform to resolve every
`registry.terraform.io/*/*` provider from the local
`_providers/` mirror and to **never** fall back to direct (network)
lookup:

```
provider_installation {
  filesystem_mirror {
    path    = "<absolute path to _providers>"
    include = ["registry.terraform.io/*/*"]
  }
  direct {
    exclude = ["registry.terraform.io/*/*"]
  }
}
```

The wrapper substitutes `<absolute path>` from
`$BUILD_WORKSPACE_DIRECTORY/bazel-bin/<rel>` at run time and exports
`TF_CLI_CONFIG_FILE` to point terraform at the substituted file.

`.terraform.lock.hcl` is rendered byte-identically to what `terraform
init` would write itself (alphabetized hashes, standard header), so
init doesn't rewrite it and the bazel output stays cache-hot across
runs.

## Bootstrap

The pin file [`/bazel/include/terraform.providers.lock.bzl`](../../../../bazel/include/terraform.providers.lock.bzl)
is committed and required for the module extension to materialize
`@terraform_providers`. On a fresh clone it's already populated for
the current provider set; only edits to `terraform.MODULE.bazel`
require a `:repin` run.

If `:repin` is invoked before any provider is declared (or after a
declaration whose hashes are missing), the module extension
soft-fails: it logs a warning and skips that provider rather than
failing the entire module-resolution phase. The missing
`@terraform_providers//:<name>` target then surfaces at consumer-side
analysis (`tf_root(providers = […])`), which is more localized than
blowing up every `bazel build` invocation.
