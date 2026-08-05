# terraform.bzl

Generate and run Terraform roots from Starlark. A root's resources are
Starlark values, not HCL: `tf_root` serialises them to `main.tf.json`, stages a
self-contained working directory beside it, and emits `plan` / `apply` /
`destroy` runnables that use a hermetic Terraform binary.

Wired into senku via `local_path_override` — see
[`bazel/include/terraform.MODULE.bazel`](../../include/terraform.MODULE.bazel).
It is not published to BCR, but nothing in it is senku-specific: the module
knows about Terraform and about GCP/Kubernetes resource shapes, and nothing
about any particular deployment.

## Setup

```python
bazel_dep(name = "terraform.bzl", version = "0.0.0")

terraform = use_extension("@terraform.bzl//terraform:extensions.bzl", "terraform")
terraform.toolchain(version = "1.14.8")
terraform.install(
    name = "terraform_providers",
    lock_file = "//:.terraform.lock.hcl",
)
use_repo(terraform, "terraform_providers", "terraform_toolchains")

register_toolchains("@terraform_toolchains//:terraform_toolchain")
```

`lock_file` is a terraform-native `.terraform.lock.hcl`. The ordinary
`terraform providers lock` flow keeps it fresh, as does Renovate's
`terraform-lockfile` manager — the module reads it rather than inventing its
own pinning format.

## A root

```python
load("@terraform.bzl", "output", "tf_root")
load("@terraform.bzl//:gcp.bzl", "google_provider", "service_account")

sa = service_account(
    name = "runner",
    account_id = "svc-runner",
    display_name = "Runtime identity",
    project = "my-project",
)

tf_root(
    name = "terraform",
    backend_bucket = "my-terraform-state",
    docs = [
        google_provider(project = "my-project"),
        sa,
        output("service_account_email", value = sa.email),
    ],
    providers = ["@terraform_providers//:google"],
)
```

```bash
bazel run //path/to:terraform.plan
bazel run //path/to:terraform.apply
bazel run //path/to:terraform.destroy
bazel run //path/to:terraform.plan -- -lock-timeout=5m   # long-tail flags
```

Resource constructors return a struct whose `.tf` is the JSON body and whose
named `attrs` are interpolation strings, so `sa.email` is
`${google_service_account.runner.email}`. Cross-resource wiring is ordinary
Starlark — no `for_each`, no `each.value`, and a typo is a Bazel analysis
error rather than a plan-time surprise.

| Load path | Surface |
| --- | --- |
| `@terraform.bzl` | `tf_root`, `deploy_task`, `ref`, `TfDeployInfo`, `TfExportsInfo`, `resource`, `output`, `var`, `variable`, `remote_state`, `merge_tf`, `tf_toolchain`, `tf_script_{test,binary}` |
| `@terraform.bzl//:gcp.bzl` | 21 GCP constructors — IAM, Cloud Scheduler, Secret Manager, Cloud SQL, GCS, Artifact Registry, WIF, monitoring |
| `@terraform.bzl//:k8s.bzl` | `kubernetes_provider`, `kubernetes_manifest` |

`resource(rtype, name, body, attrs)` covers anything without a constructor —
the constructors are conveniences over it, not a closed set.

## What ends up in bazel-bin

`tf_root` stages a complete Terraform working directory, so `terraform init`
resolves every provider locally and never reaches the network for them:

| File | Written when |
| --- | --- |
| `main.tf.json` | always — the serialised `docs` |
| `backend.tf.json` | always — GCS backend, `backend_prefix` defaults to the package path |
| `providers.tf.json`, `.terraform.lock.hcl`, `_providers/` | when `providers` is set — a filesystem mirror of the provider archives |

Provider resolution is the only part that goes offline. The GCS backend in
`backend.tf.json` is still a remote one, so `terraform init` — and every `plan`
/ `apply` / `destroy` after it — needs network access and Google credentials to
reach the state bucket.

## Deploy metadata

A repo with more than one root needs to know three things no single root can
answer alone: which roots are real, which may be applied by CI, and what has to
exist before what. `tf_root` takes these as arguments and gives them a type:
a deployable root's public target is a `tf_root_node` rule providing
`TfDeployInfo`, and the facts are its attributes.

| Argument | Default | Becomes |
| --- | --- | --- |
| `deploy` | `True` | `True` → a `tf_root_node`; `False` → a plain `filegroup` |
| `bootstrap` | `False` | the node's `bootstrap` attribute |
| `deploy_after` | `[]` | the node's `deploy_after` label list, unioned with the edges implied by any `ref()` in `docs` |
| `exports` | `{}` | the node's `exports`, readable by another root's `ref()` |

```bash
bazel query 'kind("tf_root_node|tf_deploy_task", //...)'  # every node in the graph
bazel query 'attr(bootstrap, 1, //...)'                   # what CI must not apply itself
bazel query 'attr(deploy_after, "//x:y", //...)'          # what waits on //x:y
```

Membership is the rule class rather than a flag, so there is nothing to read
and nothing to trust: a non-deployable root cannot be mistaken for a node
because it isn't one.

Edges are checked by Bazel, not by whatever walks them. `deploy_after`
requires `TfDeployInfo`, so a label that names nothing fails at load time and
one that names a non-node fails at analysis, both citing the BUILD line that
declared it. A wrong edge cannot reach a deploy, which matters because a
dropped edge is silent in exactly the way a bad deploy order is.

Nodes are **operations, not roots**. `deploy_task` puts a plain runnable in the
graph, so a step that manages no state — pushing an image, warming a cache,
running a smoke check — is an ordinary node with ordinary edges:

```python
deploy_task(
    name = "push",
    run = ":image_push",
    after = ["//infra/cloud/gcp/gar:terraform"],
)
```

That distinction is what lets an edge name the thing that actually holds the
dependency. An app's image push needs its registry to exist; the app's
Terraform does not.

The module only publishes these. It does not act on them — walking the set is
the orchestrator's job (in senku, `.aspect/{plan,apply}.axl`). That split is
deliberate: the vocabulary is useful to any driver, including a shell loop.

### `deploy` — is this a real root?

`True` means an orchestrator should include it. Set `False` for examples,
fixtures and demos: they still build and analyze, they are just never planned
or applied.

The default is `True` on purpose, because the two mistakes are not
symmetric. A root that silently *fails* to deploy is invisible — the resources
it should have created simply do not exist, and you find out when something
downstream serves the wrong thing. A root that deploys when it shouldn't fails
loudly on the very next apply. Defaulting to `True` makes the silent failure
structurally impossible.

That default is only safe if a non-deployable root cannot do damage when
someone runs its `apply` by hand. Give examples a backend bucket that cannot
resolve — `deploy = False` governs orchestrators, not `bazel run`.

### `bootstrap` — may CI apply this?

Marks a root that provisions **the CI identity itself**: the workload-identity
binding, CI's service account, the project IAM every other root leans on.

The hazard is circular. CI authenticates as a service account that this root
defines. If CI applies it and the plan is wrong — a removed binding, a renamed
account — CI can revoke its own permissions mid-apply and no longer has the
access needed to fix it. Recovery then requires a human with credentials the
pipeline never had.

So a bootstrap root is applied **locally, by a person, with their own
credentials**. Orchestrators skip it under `$CI` and walk it normally
otherwise, which keeps a developer's plan showing the whole picture. The
attribute governs what an orchestrator *discovers*, not what it is allowed to
run: senku's skips it only when walking the whole graph, and a target named
explicitly on the command line still runs under `$CI`. Same split as `deploy`
— these say what a default walk includes, and neither is an access control.

Note this is about *credentials*, not importance or ordering. A root can be
foundational without being bootstrap: the registry is a hard prerequisite for
every image-pushing root, but applying it cannot lock CI out of its own
account, so it is ordinary.

### `deploy_after` — what has to exist first?

An edge means "the resources that node creates must already exist." Nodes are
topologically sorted; ready nodes go in sorted order, so a graph with slack
always linearises the same way. A cycle is reported by name and fails.

**A deploy dependency is not a build dependency.** This is the distinction an
edge encodes, and the one that is easy to get wrong:

| A does this to B | Deploy dependency? |
| --- | --- |
| Names B's Cloud Run service in a serverless NEG | **Yes** — GCP validates the reference; a missing service serves a bare 404 |
| `load()`s a bucket *name* from B for a log filter | **No** — a filter string is never validated |

Both look identical in the build graph: A imports a constant from B. Whether
the reference is checked is a property of the provider's API, not of Starlark,
so it cannot be read off `load()` edges and has to be stated.

**Derive edges; do not hand-maintain them.** A list nobody has to remember to
update cannot drift, and that is the whole point — this module grew deploy
metadata because a hand-maintained deploy list silently lost an entry. In order
of preference:

- **`ref()` the value you need** (below). The reference *is* the edge, so
  there is nothing to keep in agreement with anything.
- **Hand the dependency out with the address.** Where a value never reaches
  the generated JSON — a registry hostname, which is a Bazel rule attribute —
  export a descriptor carrying the address *and* the root that provisions it,
  and have the consuming macro turn the latter into its edge. A caller then
  cannot resolve where to push without also learning what it must wait for.
- **Declare `deploy_after` by hand.** For a dependency no value passes
  through at all.

See [ADR 0008](../../../docs/adr/0008-derived-terraform-deploy-set.md) for the
alternatives considered — integer tiers, inferring order from the `load()`
graph, and a degenerate `tf_root` in place of `deploy_task`.

## Cross-root references

A root publishes build-time constants with `exports`; another root names one
with `ref`, and the deploy edge comes from the same token.

```python
# //apps/cluedo-bayes:BUILD — the producer
tf_root(
    name = "terraform",
    exports = {"service_name": SERVICE_NAME},
    docs = [...],
)

# //infra/cloud/gcp/lb:defs.bzl — the consumer
"service_name": ref("//apps/cluedo-bayes:terraform", "service_name")
```

The LB declares no `deploy_after`: `tf_root` scans the document it is about to
serialise, finds the reference, and takes the edge from it. The value routed to
and the root waited on cannot come to disagree, because there is only one of
them. `ref()` returns a sentinel string, so it survives ordinary string
building — concatenation, `%`, `.format()` — and is substituted by an
analysis-time action before Terraform sees anything.

Everything is checked at build time. A label naming nothing fails at load; a
label naming a non-root fails on the missing provider; an export name that
does not exist fails with the available names listed; a root referencing its
own exports fails with that said plainly rather than as a dependency cycle.

An export is usually a literal, but it does not have to be: a node can export
the *contents of a file* the build produces. senku's image pushes do exactly
that — the digest-pinned URI is only known once the image is built, so
`registry_push` publishes it as a file-backed export and a root names it with
an ordinary `ref`. Literals are placed at analysis time, file-backed values
during the action; `ref` does not distinguish them, because both are settled
before Terraform runs and that is the only property that matters.

**Exports are not Terraform outputs**, and the distinction is the whole reason
they are named differently:

| | `output()` in `docs` | `exports` |
| --- | --- | --- |
| Resolved | at apply, by Terraform | at analysis, by Bazel |
| May be dynamic | yes — an allocated IP | **no** — literals only |
| Read with | `remote_state()` | `ref()` |
| In state | yes | no |

`tf_root` rejects an export containing `${`, pointing at `output()` +
`remote_state()`, which remain the mechanism for values that only exist after
an apply.

Two limits worth knowing. A reference only resolves inside the document of the
root that makes it — a value needed in a Bazel rule attribute (an
`image_push`'s registry) or in Starlark control flow (a dict key, a loop
bound) has to stay an ordinary `load()`ed constant. And a `ref` always implies
an edge, so a value the provider never validates — a bucket name in a log
filter — should keep using `load()`, which claims nothing.

## Layout

| Path | Contents |
| --- | --- |
| `terraform/defs.bzl` | `tf_root` and the value helpers — `resource`, `output`, `var`, `variable`, `remote_state`, `merge_tf` |
| `terraform/deploy.bzl` | `TfDeployInfo` and the node rules — `tf_root_node`, `tf_deploy_task`, `deploy_task` |
| `terraform/refs.bzl` | `TfExportsInfo`, `ref` and the sentinel resolver — cross-root values and the edges they imply |
| `terraform/rule.bzl` | `tf_runner` — resolves the toolchain at analysis time and bakes the paths into a wrapper, so direct-spawn callers work without `bazel run` |
| `terraform/extensions.bzl` | The `terraform` module extension: `toolchain()` and `install()` |
| `terraform/resources/` | Provider-specific constructors (`gcp.bzl`, `k8s.bzl`) |
| `terraform/lockfile.bzl`, `provider.bzl`, `platforms.bzl` | Provider pinning, multi-platform hashes, the filesystem mirror |
| `terraform/hcl.bzl`, `hcl_test.bzl`, `lint.bzl` | HCL emission and lint/test wrappers |
| `terraform/toolchain/` | Terraform CLI toolchain |
| `gcp.bzl`, `k8s.bzl`, `terraform.bzl` (root) | Re-export shims giving consumers short load paths |

## Trade-offs

Worth knowing before adopting, and unchanged by anything above:

- `terraform fmt`, `tflint` and IDE plugins target HCL, not generated JSON.
- Terraform errors cite JSON line numbers in `bazel-bin/...`, not hand-written
  source.
- Off-the-shelf registry modules still work through `module {}` blocks, but
  their examples do not translate line for line.
- State migration stays manual: if a resource's address changes in the
  generated JSON, that is a `terraform state mv`.
