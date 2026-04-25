# Infra as Starlark — Plan

A plan to move Terraform from hand-written HCL + GitHub Actions choreography to
Starlark-generated `.tf.json` driven by Bazel. The DAG between roots becomes a
build artifact, not a CI YAML quirk. Local `bazel run` and CI run the same
command.

> Status: proposal. Nothing in this doc is built yet. The current state is
> hand-written HCL in `infra/cloud/gcp/{gar,lb}` and `oci/cmd/registry/terraform`,
> with `oci/cmd/registry/deploy.sh` orchestrating the registry root.

## Motivation

We now have three Terraform roots with a real dependency graph:

```
infra/cloud/gcp/gar           # Artifact Registry repo
        │
        ▼
oci/cmd/registry/terraform    # Cloud Run services per region (image lives in GAR)
        │
        ▼
infra/cloud/gcp/lb            # Global LB fronting the Cloud Run services
```

The graph will keep growing. Each addition currently costs:

- A new `terraform` job (or matrix entry) in `.github/workflows/ci.yaml`.
- A `terraform_remote_state` data block in every consumer root.
- Hand-maintained `needs:` ordering in the workflow.
- Drift between `bazel run :deploy.sh` style local flow and CI's inlined steps.

In a Bazel monorepo, the obvious move is to express the DAG in Bazel and stop
writing it twice. Terragrunt would solve it with a parallel CLI and DSL — fine
for non-monorepo shops, weird for us. Generating `.tf.json` from Starlark keeps
Terraform doing what it's good at (resource graph, providers, state) while
Bazel does what it's good at (dependency graph, hermetic toolchains, one
command for everything).

## Approach

Three composable primitives, each ~150 lines or less:

1. **Resource constructors** — Starlark functions returning a struct with `.tf`
   (the JSON for the resource) and one attribute per cross-resource reference.
2. **`tf_root`** — macro that takes a list of resources, merges them, writes
   `.tf.json` files, and emits `:plan` / `:apply` / `:destroy` runnable targets.
3. **`tf_dag`** — macro that takes a topologically ordered list of `tf_root`s
   and emits one runner that walks them.

Terraform's interpolation language stays — `${...}` strings flow through the
JSON unchanged. Starlark only handles things resolvable at generation time
(loops, defaults, shared constants); cross-resource refs and provider state
remain Terraform's job.

## Architecture

```
BUILD files (Starlark)
    │
    ├── service_account(...)         ── struct(.tf, .email, .id, ...)
    ├── cloud_run_service(...)       ── struct(.tf, .uri, .id, ...)
    ├── var("project_id")            ── "${var.project_id}"
    ├── remote_state(...)            ── struct(.tf, .<output>, ...)
    │
    └── tf_root(name, docs, ...)
            │
            ├── write_file ──► main.tf.json + backend.tf.json
            └── sh_binary  ──► :<name>.plan / .apply / .destroy

tf_dag(name, roots = [...])
            │
            └── sh_binary  ──► walks the chain, fail-fast
```

## Resource constructors

Each constructor returns a `struct` with two layers: the JSON body keyed under
`.tf`, and one field per attribute that downstream resources can reference.

```python
# devtools/build/tools/tf/defs.bzl

def _resource(rtype, name, body, attrs = ()):
    return struct(
        tf = {"resource": {rtype: {name: body}}},
        addr = "%s.%s" % (rtype, name),  # for depends_on
        **{a: "${%s.%s.%s}" % (rtype, name, a) for a in attrs}
    )

def service_account(name, project, account_id, display_name):
    return _resource(
        rtype = "google_service_account",
        name = name,
        body = {
            "project": project,
            "account_id": account_id,
            "display_name": display_name,
        },
        attrs = ["email", "id", "name", "unique_id", "member"],
    )
```

Cross-root reads piggy-back on the same shape via `terraform_remote_state`:

```python
def remote_state(name, prefix, outputs):
    return struct(
        tf = {"data": {"terraform_remote_state": {name: {
            "backend": "gcs",
            "config": {"bucket": "senku-prod-terraform-state", "prefix": prefix},
        }}}},
        **{o: "${data.terraform_remote_state.%s.outputs.%s}" % (name, o)
           for o in outputs}
    )
```

Module invocations look the same — the `.tf` body uses `module` instead of
`resource`, and the attribute refs become `module.X.outputs.Y`.

## tf_root

```python
load("@bazel_skylib//rules:write_file.bzl", "write_file")

def tf_root(name, docs, backend_prefix, pre_apply = [], visibility = None):
    body = _merge(*[d.tf if hasattr(d, "tf") else d for d in docs])
    backend = {"terraform": {"backend": {"gcs": {
        "bucket": "senku-prod-terraform-state",
        "prefix": backend_prefix,
    }}}}

    write_file(name = name + ".main",    out = name + "/main.tf.json",
               content = [json.encode_indent(body)])
    write_file(name = name + ".backend", out = name + "/backend.tf.json",
               content = [json.encode_indent(backend)])

    generated = [name + ".main", name + ".backend"]

    for verb in ("plan", "apply", "destroy"):
        native.sh_binary(
            name = "%s.%s" % (name, verb),
            srcs = ["//devtools/build/tools/tf:run.sh"],
            args = [verb, "$(rootpath %s)" % generated[0]],
            data = generated + pre_apply + ["@terraform//:terraform"],
            env = {
                "TF_BIN": "$(rootpath @terraform//:terraform)",
                "ROOT_NAME": name,
                "PRE_APPLY": " ".join(["$(rootpath %s)" % p for p in pre_apply]),
            },
            tags = ["manual"],
            visibility = visibility,
        )
```

`run.sh` copies the generated files out of the read-only runfiles tree into
`~/.cache/senku-tf/<root>/`, runs each `pre_apply` target (image push, tfvars
materialization), then `terraform init` + the requested verb. State stays in
GCS — Bazel never tries to own it.

## tf_dag

```python
def tf_dag(name, roots, verb = "apply"):
    targets = ["%s.%s" % (r, verb) for r in roots]
    native.sh_binary(
        name = name,
        srcs = ["//devtools/build/tools/tf:dag.sh"],
        args = ["$(rootpath %s)" % t for t in targets],
        data = targets,
        tags = ["manual"],
    )
```

```python
# infra/BUILD
tf_dag(
    name = "apply_all",
    roots = [
        "//infra/cloud/gcp/gar:terraform",
        "//oci/cmd/registry:terraform",
        "//infra/cloud/gcp/lb:terraform",
    ],
)

tf_dag(
    name = "plan_all",
    verb = "plan",
    roots = [...],  # same list
)
```

One ordered list, one place to maintain it. Auto-detection from a `requires`
field on each `tf_root` is a future cleanup, not worth the complexity at three
roots.

## Worked example: registry

`oci/cmd/registry/BUILD` after migration:

```python
load("//devtools/build/tools/tf:defs.bzl",
     "service_account", "cloud_run_service", "tf_root", "var")

REGIONS = ["us-central1", "europe-west3", "asia-northeast1"]

sa = service_account(
    name = "registry",
    project = var("project_id"),
    account_id = "svc-registry",
    display_name = "Runtime identity for registry",
)

services = [
    cloud_run_service(
        name = "registry_%s" % r.replace("-", "_"),
        project = var("project_id"),
        region = r,
        image = var("image"),
        service_account_email = sa.email,         # <-- the cross-resource ref
        args = [
            "--upstream=ghcr.io",
            "--repository-prefix=arkeros/senku",
        ],
        scaling = {"min_instance_count": 0, "max_instance_count": 3},
        ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER",
    )
    for r in REGIONS
]

tf_root(
    name = "terraform",
    backend_prefix = "oci/cmd/registry/terraform",
    docs = [sa] + services + [
        # outputs consumed by //infra/cloud/gcp/lb
        {"output": {"lb_backends": {"value": {
            "registry": {
                "service_name": "registry",
                "regions": REGIONS,
                "paths": ["/v2/*"],
            },
        }}}},
    ],
    pre_apply = [
        ":image_push_gar",   # already exists in BUILD
        ":image_tfvars",     # already exists in BUILD
    ],
)
```

`bazel run //oci/cmd/registry:terraform.apply` replaces `deploy.sh`. The push
+ tfvars + apply chain is encoded in `pre_apply`, no separate shell script.

## Migration plan

The plan is *incremental*. Every step lands a green commit; we can stop at any
step and still have a working repo.

### Step 1 — Land the primitives, no callers

- Add `devtools/build/tools/tf/defs.bzl` with `_resource`, `tf_root`, `tf_dag`,
  `var`, `remote_state`.
- Add `devtools/build/tools/tf/run.sh` and `dag.sh`.
- Pin a `@terraform//` toolchain via `bzlmod` (replaces
  `hashicorp/setup-terraform@v4` in CI).
- Unit test by generating a no-op `tf_root` with a single null resource and
  confirming `bazel run :foo.plan` exits 0 against a temp backend.

No production roots touched. Reviewable in isolation.

### Step 2 — Migrate the leaf root (`infra/cloud/gcp/gar`)

Smallest root, no consumers, trivial to roll back. Convert
`infra/cloud/gcp/gar/main.tf` to a `tf_root` definition in
`infra/cloud/gcp/gar/BUILD`. Delete the `.tf` files.

CI changes: replace the matrix entry with `bazel run //infra/cloud/gcp/gar:terraform.plan`
on PR / `.apply` on push.

Validation: `terraform plan` shows no diff vs the deployed state.

### Step 3 — Migrate `oci/cmd/registry/terraform`

Larger, has the image-push wrinkle. The existing `image_push_gar` and
`image_tfvars` Bazel targets stay; we plug them in via `pre_apply` instead of
calling `deploy.sh`.

Delete `deploy.sh` once `bazel run //oci/cmd/registry:terraform.apply` is
proven to do the same thing.

### Step 4 — Migrate `infra/cloud/gcp/lb`

This is the hairiest because the LB stack reads other roots' outputs via
`terraform_remote_state`. Convert each remote-state block to `remote_state(...)`
in Starlark; the references resolve to the same `${data.terraform_remote_state...}`
strings.

### Step 5 — Add `tf_dag`, collapse CI

```yaml
# .github/workflows/ci.yaml
infra:
  runs-on: ubuntu-latest
  permissions: { contents: read, id-token: write, packages: write, pull-requests: write }
  steps:
    - checkout (fetch-depth: 0)
    - setup bazel
    - auth to GCP (WIF)
    - login to GAR (docker)
    - if PR:    bazel run //infra:plan_all
    - if push:  bazel run //infra:apply_all
```

The three jobs (`gar`, `registry`, `lb`) collapse to one. The plan-comment
logic moves out of the workflow into a Bazel-side reporter that walks the
roots and posts one comment per root with the plan diff.

### Step 6 — Optional: add `infra/cloud/gcp/lb/examples/hello`

If we already have it (the existing `lb/main.tf` references `examples`), bring
it under `tf_root` too. Otherwise skip.

## Trade-offs

**Wins**
- One DAG declaration. CI doesn't re-encode it.
- Hermetic Terraform binary version (toolchain), no `setup-terraform` race.
- Local and CI run the same command. Reproducing a CI failure is `bazel run`.
- Reuse: `cloud_run_service(...)` defined once, used from registry and any
  future service. Resource-level abstractions instead of HCL modules.
- Loops in Starlark. Region fan-out doesn't need `for_each` + `each.value`.

**Losses**
- `terraform fmt` / `tflint` / IDE plugins target HCL, not generated JSON.
- Stack traces from terraform errors point at JSON line numbers in
  `bazel-bin/...`, not at hand-written source.
- Off-the-shelf modules from the registry expect HCL ergonomics. They still
  work via `module {}` blocks, but examples don't translate 1:1.
- New contributors learn one more rule before they can edit infra.

**Things this doesn't fix**
- State migration is still manual (`terraform state mv` if resource addresses
  change in the JSON output).
- Cross-root output reads still go through GCS state. No way to avoid the
  one-cycle-late freshness issue on PR plans.
- `terraform_remote_state` requires the upstream root to be applied at least
  once before downstream plans work. First-time bootstrapping needs ordered
  applies.

## Open questions

1. **Plan output for PRs.** The current CI posts each root's plan as a PR
   comment. Where does this live in the new world? Options: keep posting from
   GHA (simplest), move into `tf_root` as a `plan_json` target (more
   consistent), build a dedicated reporter rule (highest investment).

2. **Auto-DAG vs explicit list.** `tf_dag` currently takes a hand-ordered list.
   At what root count is it worth a Bazel aspect that walks `requires` fields
   and topo-sorts? Best guess: ~7+ roots.

3. **Multi-environment.** Today there's one `senku-prod`. If we add `staging`,
   does each root take an `env` parameter and emit one tf_root per env, or do
   we keep one root per env on disk? Affects how `var()` and backend prefixes
   are scoped.

4. **Drift detection.** Once `bazel run //infra:apply_all` is the deploy path,
   running `bazel run //infra:plan_all` on a cron and alerting on non-empty
   diffs gives free drift detection. Worth wiring up; not urgent.

5. **rules_terraform vs homegrown.** Public `rules_terraform` implementations
   are mostly abandoned. The 150-line homegrown approach above costs less than
   evaluating a third-party rule set. Reconsider if our needs grow past what
   `tf_root` + `tf_dag` covers (e.g., per-resource targeted apply, plan
   caching keyed on TF state).

## Why not Terragrunt

Terragrunt solves the same problem with a parallel CLI (`terragrunt run-all`)
and a parallel DSL (`terragrunt.hcl` per root). In a non-monorepo shop, that's
the right answer. In this repo:

- A second build orchestrator alongside Bazel doubles the conceptual surface.
- `terragrunt.hcl`'s `dependency` blocks duplicate what Bazel's `deps` already
  expresses.
- The "DRY backend" benefit is five lines of Starlark.
- Terragrunt's plan/apply doesn't compose with `image_push_gar` and `image_tfvars` — we'd still need a wrapper script.

Terragrunt is a fine tool. It's just not the tool that fits the rest of the
repo.

## Why not CDKTF

CDKTF compiles TypeScript/Python/Go/Java to `.tf.json`. Same emission target,
larger ecosystem, real type system. The cost is adopting a non-Starlark
language for infra in a Bazel monorepo where everything else (frontend
framework, Python tooling, image rules) is Starlark-defined. The type-safety
win from CDKTF is real but smaller than the consistency win from staying in
one language.

## File layout (proposed)

```
devtools/build/tools/tf/
├── defs.bzl              — tf_root, tf_dag, _resource, var, remote_state
├── resources/
│   ├── gcp.bzl           — service_account, cloud_run_service, ...
│   └── google_compute.bzl — backend_service, url_map, ...
├── run.sh                — per-root plan/apply runner
├── dag.sh                — DAG walker
└── BUILD                 — exposes the toolchain + scripts

infra/
├── BUILD                 — tf_dag(plan_all, apply_all)
└── cloud/gcp/
    ├── gar/BUILD         — tf_root(...)
    └── lb/BUILD          — tf_root(...)

oci/cmd/registry/
└── BUILD                 — tf_root(...) with pre_apply = [image_push_gar, image_tfvars]
```

The `resources/` split keeps GCP-specific constructors out of the core rules
so `tf_root` itself stays provider-agnostic.

## Decision points before starting

- Is the migration worth it now, or wait until we have 5+ roots?
- Are we OK losing `terraform fmt` and IDE HCL tooling?
- Do we want to keep `deploy.sh` as an escape hatch during the transition, or
  delete it as part of step 3?

If the answers are "yes, OK, delete," start with step 1.
