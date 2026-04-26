# Infra as Starlark

The migration from hand-written HCL + GitHub Actions choreography to
Starlark-generated `.tf.json` driven by Bazel. The DAG between roots is a
build artifact, not a CI YAML quirk. Local and CI run the same commands.

> Status: implemented. This doc captures the design and the migration trail
> together — both as a record of the decisions made (Terragrunt rejection,
> CDKTF rejection, `tf_dag` rejection in favour of Aspect CLI, etc.) and as
> the reference for adding the next root. The three production roots
> (`infra/cloud/gcp/gar`, `oci/cmd/registry`, `infra/cloud/gcp/lb`) are all
> on the new path; the HCL form of bifrost's `service_cloudrun` module
> (and its standalone `examples/hello` consumer) was deleted as part of
> the migration — Starlark is the single source of truth.

## Motivation

We now have four Terraform roots with a real dependency graph:

```
infra/cloud/gcp/ci            # CI bootstrap: WIF + GHA SA + project IAM (apply locally first)
        │
        ▼
infra/cloud/gcp/gar           # Artifact Registry repo
        │
        ▼
oci/cmd/registry              # Cloud Run services per region (image lives in GAR)
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

Two composable primitives, each ~150 lines or less:

1. **Resource constructors** — Starlark functions returning a struct with `.tf`
   (the JSON for the resource) and one attribute per cross-resource reference.
2. **`tf_root`** — macro that takes a list of resources, merges them, writes
   `.tf.json` files, and emits `:plan` / `:apply` / `:destroy` runnable targets.

Terraform's interpolation language stays — `${...}` strings flow through the
JSON unchanged. Starlark only handles things resolvable at generation time
(loops, defaults, shared constants); cross-resource refs and provider state
remain Terraform's job.

Cross-root sequencing (apply gar, then registry, then lb) is *not* Bazel's
job. Per Aspect's [outside-of-Bazel pattern](https://blog.aspect.build/outside-of-bazel-pattern),
multi-process orchestration belongs in the task layer. We adopt the
[Aspect CLI](https://docs.aspect.build/cli/) for that: `.aspect/plan.axl`
and `.aspect/apply.axl` define `aspect plan` / `aspect apply` tasks that
walk the root list in dependency order. Same command runs locally and (with
aspect-cli installed) in CI. Bazel owns the build graph; aspect-cli
orchestrates the runs.

## Architecture

```
BUILD files (Starlark)
    │
    ├── service_account(...)         ── struct(.tf, .email, .id, ...)
    ├── service_cloudrun(...)       ── struct(.tf, .uri, .id, ...)
    ├── var("project_id")            ── "${var.project_id}"
    ├── remote_state(...)            ── struct(.tf, .<output>, ...)
    │
    └── tf_root(name, docs, ...)
            │
            ├── write_file ──► main.tf.json + backend.tf.json
            └── sh_binary  ──► :<name>.plan / .apply / .destroy
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

## Cross-root orchestration (Aspect CLI, not Bazel)

We considered a `tf_dag` macro that walked N roots in order via `bazel run`.
Aspect Build's [outside-of-Bazel pattern](https://blog.aspect.build/outside-of-bazel-pattern)
named exactly the smell: *multi-process orchestration belongs in the task
layer, not the build core*. We agreed and dropped it.

Sequencing now lives in `.aspect/{plan,apply}.axl`. `stdlib.axl` defines
`TF_ROOTS` (the ordered list). The tasks are thin Starlark wrappers around
`bazel run <root>.{plan,apply}`:

```python
# .aspect/stdlib.axl
TF_ROOTS = [
    "//infra/cloud/gcp/gar:terraform",
    "//oci/cmd/registry:terraform",
    "//infra/cloud/gcp/lb:terraform",
]
```

```bash
aspect apply                                  # all roots, gar → registry → lb
aspect apply //oci/cmd/registry:terraform     # one root
aspect plan                                   # all roots
aspect plan //infra/cloud/gcp/lb:terraform    # one root
```

CI's `needs:` graph mirrors `TF_ROOTS` for per-step UI (per-root PR plan
comments, per-root retries). Same source of truth, two surfaces.

## Worked example: registry

`oci/cmd/registry/BUILD` after migration:

```python
load("//devtools/build/tools/tf:defs.bzl",
     "service_account", "service_cloudrun", "tf_root", "var")

REGIONS = ["us-central1", "europe-west3", "asia-northeast1"]

sa = service_account(
    name = "registry",
    project = var("project_id"),
    account_id = "svc-registry",
    display_name = "Runtime identity for registry",
)

services = [
    service_cloudrun(
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

- Add `devtools/build/tools/tf/defs.bzl` with `resource`, `tf_root`, `var`,
  `remote_state`.
- Add `devtools/build/tools/tf/run.sh`.
- Add `devtools/build/tools/tf/render.bzl` for build-time digest substitution.
- Pin a `@terraform//` toolchain via `bzlmod` (replaces
  `hashicorp/setup-terraform@v4` in CI).
- Snapshot-test by generating a no-op `tf_root` and `diff_test`-ing the JSON.

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

This is the hairiest because the LB aggregates outputs from every backend
service. Rather than reading those via `terraform_remote_state` at plan time,
have each service expose an `LB_BACKEND` constant from its own `defs.bzl` —
the LB root imports them directly via Starlark `load()`. Same content, no
runtime indirection, fail-fast at Bazel build time.

### Step 5 — Adopt Aspect CLI for orchestration + wire CI

Add `.aspect/{stdlib,plan,apply}.axl`:

- `stdlib.axl` exports `TF_ROOTS` (ordered list) and a small `bazel_run`
  helper that auto-sets `TF_AUTO_APPROVE` when `$CI` is set.
- `plan.axl` defines `aspect plan [<target>]` — runs all roots or one.
- `apply.axl` defines `aspect apply [<target>]` — runs all in order, with
  `--stamp` (registry's image tags need it).

Three new GHA jobs (`gar`, `registry`, `lb`), each invoking
`bazel run //path:terraform.{plan,apply}` (or `aspect <command>` if
aspect-cli is set up in CI). Plans run in parallel on PR, applies chain
via `needs:` on push.

### Step 6 — Delete the HCL twin of `service_cloudrun` and its example

Once the registry root is fully on the Starlark `service_cloudrun` macro,
the parallel HCL implementation becomes drift-prone. Delete it:

- `devtools/bifrost/modules/service_cloudrun/{main,outputs,variables,versions}.tf`
- `devtools/bifrost/modules/service_cloudrun/examples/`
- `infra/cloud/gcp/lb/examples/hello/` (the only HCL consumer)

Bifrost's `service_cloudrun` then exists only in `defs.bzl` — Starlark is the
single source of truth.

### Step 7 — Multi-environment

Design captured in the "Multi-environment" section below. Not implemented
in this PR — defer until the first non-prod env is actually needed.

## Trade-offs

**Wins**
- One DAG declaration. CI doesn't re-encode it.
- Hermetic Terraform binary version (toolchain), no `setup-terraform` race.
- Local and CI run the same command. Reproducing a CI failure is `bazel run`.
- Reuse: `service_cloudrun(...)` defined once, used from registry and any
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

## Multi-environment

Today there's one env: `senku-prod`. To add `senku-staging` (or any other),
the shape is **macro factory per root** + **env-keyed dict in `defs.bzl`** +
**env-aware `aspect` task**.

### Shape

Each root's `defs.bzl` declares the per-env knobs as a dict, and exports a
macro that emits one `tf_root` per env. The BUILD just iterates:

```python
# infra/cloud/gcp/gar/defs.bzl

load(...)

ENVS = {
    "prod":    struct(project = "senku-prod",    location = "europe", repository_id = "containers"),
    "staging": struct(project = "senku-staging", location = "europe", repository_id = "containers"),
}

# Derived strings, keyed by env. Image-push rules consume these.
GAR_REGISTRY          = {e: cfg.location + "-docker.pkg.dev"        for e, cfg in ENVS.items()}
GAR_REPOSITORY_PREFIX = {e: cfg.project  + "/" + cfg.repository_id  for e, cfg in ENVS.items()}

def gar_root(env):
    cfg = ENVS[env]
    api = project_service(
        name = "artifactregistry_" + env,
        project = cfg.project,
        service = "artifactregistry.googleapis.com",
        disable_on_destroy = False,
    )
    repo = artifact_registry_repository(
        name = "containers_" + env,
        project = cfg.project,
        location = cfg.location,
        repository_id = cfg.repository_id,
        ...
        depends_on = [api.addr],
    )
    tf_root(
        name = env + "_terraform",
        backend_prefix = "infra/cloud/gcp/gar/" + env,    # one prefix per env
        docs = [google_provider(project = cfg.project), api, repo, ...],
        required_providers = {...},
        visibility = ["//visibility:public"],
    )
```

```python
# infra/cloud/gcp/gar/BUILD
load(":defs.bzl", "ENVS", "gar_root")

[gar_root(env) for env in ENVS]
```

Result: targets `//infra/cloud/gcp/gar:prod_terraform.{plan,apply}` and
`//infra/cloud/gcp/gar:staging_terraform.{plan,apply}`. Each has its own
state under its own backend prefix; same code path otherwise.

### Aspect CLI

`TF_ROOTS` becomes a dict keyed by env; `aspect apply` / `aspect plan` accept
an `--env` flag (defaulting to `prod`):

```python
# .aspect/stdlib.axl
TF_ROOTS = {
    "prod": [
        "//infra/cloud/gcp/gar:prod_terraform",
        "//oci/cmd/registry:prod_terraform",
        "//infra/cloud/gcp/lb:prod_terraform",
    ],
    "staging": [
        "//infra/cloud/gcp/gar:staging_terraform",
        "//oci/cmd/registry:staging_terraform",
        "//infra/cloud/gcp/lb:staging_terraform",
    ],
}
```

```bash
aspect apply              # default: prod
aspect apply --env=staging
aspect plan  --env=staging //oci/cmd/registry:staging_terraform
```

### Image push: shared vs per-env GAR

Two valid options:

1. **Shared GAR (one source of truth).** Push images once to `senku-prod`'s
   GAR; staging Cloud Run pulls from the same registry. Simpler, cheaper,
   but blurs the env boundary (a botched prod push could affect staging).
2. **Per-env GAR.** Each env has its own `gar` root + its own image_push
   target. `service_cloudrun` references the env's GAR. Strict isolation
   but doubles the push cost and makes the build's image-push step env-aware.

Recommend **shared GAR until staging's isolation requirements force it
otherwise** — for most multi-env setups, the registry isn't the
trust-boundary anyway (the Cloud Run service identity and IAM bindings
are).

### Cross-root constants per env

`LB_BACKEND` (currently a single dict in `oci/cmd/registry/defs.bzl`) becomes
keyed by env, like `GAR_REGISTRY`:

```python
# oci/cmd/registry/defs.bzl
LB_BACKEND = {
    env: {
        "service_name": "registry",
        "regions": sorted(REGIONS[env]),
        "paths":   ["/v2/*"],
    }
    for env in ENVS
}
```

Then lb's `BACKENDS` is also env-keyed, and lb's macro emits per-env LB stacks
that consume the right slice.

### What this *doesn't* solve

- **Org-policy or quota differences between envs** — projects might have
  different ingress policies, region availability, etc. Those leak into the
  per-env `cfg` struct; no uniform answer.
- **Promotion flow.** "Apply to staging, soak, then apply to prod" is a CI
  question, not an infra-as-Starlark question. Aspect tasks would call out to
  whatever promotion machinery lives elsewhere.

### When to do it

Don't pre-emptively. Land it the first PR that actually adds a non-prod env;
trying to retro-fit later is cheap because the call sites are concentrated in
each root's `defs.bzl`.

## Open questions

1. **Plan output for PRs.** The current CI posts a single combined plan as a
   PR comment. At ~3 roots that's fine; at 6+ reviewers will want per-root
   threading. Either (a) move per-root output capture into `aspect plan` and
   have CI fan out into per-root comments, or (b) keep a matrix of CI jobs
   each calling `aspect plan <one-root>` and posting separately.

2. **Drift detection.** Run all `:terraform.plan` targets on a cron and alert
   on non-empty diffs. Free drift detection. Worth wiring up; not urgent.

3. **rules_terraform vs homegrown.** Public `rules_terraform` implementations
   are mostly abandoned. The 150-line homegrown approach above costs less than
   evaluating a third-party rule set. Reconsider if our needs grow past what
   `tf_root` covers (e.g., per-resource targeted apply, plan caching keyed on
   TF state).

4. **State migrations as code.** Today, `terraform state mv` invocations are
   ad-hoc (replay-script in PR bodies). A `migrations/<date>-<purpose>.sh`
   convention with `tags = ["manual"]` Bazel runners would make them
   reviewable and replayable.

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
├── defs.bzl              — tf_root, resource, var, remote_state
├── render.bzl            — IMAGE_URI sentinel + render_main_with_image
├── resources/
│   └── gcp.bzl           — service_account, project_service, ...
├── run.sh                — per-root plan/apply runner
└── BUILD                 — exposes the toolchain + script

devtools/bifrost/modules/
└── service_cloudrun/
    └── defs.bzl          — service_cloudrun Starlark macro

infra/cloud/gcp/
├── gar/{BUILD,defs.bzl}  — tf_root + GAR identity constants
└── lb/{BUILD,defs.bzl}   — tf_root + LB resources

oci/cmd/registry/
└── {BUILD,defs.bzl}      — tf_root with image_push, registry identity + LB_BACKEND
```

The `resources/` split keeps GCP-specific constructors out of the core rules
so `tf_root` itself stays provider-agnostic.

## Decision points before starting

- Is the migration worth it now, or wait until we have 5+ roots?
- Are we OK losing `terraform fmt` and IDE HCL tooling?
- Do we want to keep `deploy.sh` as an escape hatch during the transition, or
  delete it as part of step 3?

If the answers are "yes, OK, delete," start with step 1.
