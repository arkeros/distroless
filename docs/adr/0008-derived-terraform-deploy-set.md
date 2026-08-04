# The deploy DAG is derived from the build graph, not listed

`aspect plan` / `aspect apply` discover what to walk by querying the build graph for tags each node publishes about itself — membership (`tf-deploy`), how to run it (`tf-kind`), credential tier (`tf-bootstrap`) and its edges (`tf-after=<label>`) — then topologically sort them. The hand-maintained `TF_ROOTS` list in `.aspect/stdlib.axl` is deleted. Joining the deploy DAG is now the same act as declaring the node, so there is nothing to register and nothing to forget.

Nodes are **operations, not roots**. A Terraform root is one kind of node; a container-image push is another. That is what lets an edge point at the thing that actually holds the dependency: an app's image push needs the registry to exist, while the app's Terraform does not.

`aspect dag` prints the resulting order without running anything.

## Why

`TF_ROOTS` was an ordered list of ten entries, edited roughly once per new app, carrying three facts the orchestrator needed. It drifted the first time it was load-bearing under time pressure: `//apps/spaghetti-duel:terraform` was added to the repo without a corresponding entry. Nothing caught it — the app built, tested, reviewed, merged and passed CI. The load balancer root *was* in the list, so it applied happily and created a certificate, a host rule, a backend service and a serverless NEG naming a Cloud Run service that nothing would ever create.

That failure is quiet in a specific and expensive way: **a serverless NEG whose backing Cloud Run service does not exist serves a bare `404` rather than failing.** No error, no unhealthy backend, no signal in any dashboard — just a 404 with no backend headers, indistinguishable from a missing asset. Diagnosis cost an afternoon and went down two wrong paths (a mid-issuance TLS certificate, then a suspected routing bug) before arriving at "the service was never created."

A list that must be edited in lockstep with another file is a synchronization problem, and this repo already had evidence it loses that game: `.aspect/README.md` documented a `BOOTSTRAP_ROOTS` constant that had been refactored away into `struct(bootstrap = True)` some time earlier, and nobody noticed.

## What each fact rides on

| Fact | Where it lives | Cost of adding a root |
| --- | --- | --- |
| Membership | `deploy = True`, the `tf_root` default | none |
| Bootstrap tier | `bootstrap = True` | two call sites, both pre-existing |
| Push → registry | `registry_push`, from the registry descriptor it is handed | none |
| Root → its own push | `tf_root_with_image`, which already receives the label | none |
| LB → its backends | derived from `BACKENDS` | none — a backend that routes here is necessarily in that dict |

`deploy` defaults to **True** deliberately. The two mistakes are not symmetric: a root that silently fails to deploy is a 404 that surfaces hours later in a browser, while a root that deploys when it shouldn't fails loudly on the next apply. Defaulting to in makes the silent failure structurally impossible and leaves only the loud one — but see *Prerequisite* below, because that reasoning does not hold on its own.

## Every edge is derived

No edge is hand-written, which is the property that matters: a list nobody maintains cannot drift.

- **A push waits for its registry.** `registry_push` is handed a *registry descriptor* — host, repository prefix, and the root that provisions it — and turns the `root` field into its edge. The address and the dependency arrive together, so a caller cannot resolve where to push without also learning what it has to wait for.
- **A root waits for its own push.** `tf_root_with_image` already receives the `image_push` label in order to substitute the digest; it declares the edge from that.
- **The load balancer waits for its backends.** Derived from `BACKENDS`. A backend that routes here is necessarily in that dict, and `_normalize` rejects one that omits its `root`.

Which yields, with nothing declared per call site:

```
gar → {6 image pushes} → {6 roots} → lb
```

**Fail-fast falls out of the shape.** Every push has only the registry as a predecessor, so all six sort ahead of every Terraform apply. A registry problem now fails before any infrastructure is mutated, instead of half-way through a deploy.

### Rejected: tiers

An earlier revision of this ADR used integer `deploy_tier` buckets. Tiers impose a total order on what is only a partial one, the number says *what* but never *why*, and inserting a layer means renumbering. They also cannot express operation granularity — the whole point below.

### Rejected: infer the order from the `load()` graph

This nearly works, and it is worth recording *why it was not obviously wrong*. Topologically sorting `buildfiles(deps(<root>))` over the roots produces the correct order today, with no annotations at all: apps load the registry's constants, the LB loads every backend's, so the edges are already there.

Two reasons against it:

**It is package-granular, so it attributes the dependency to the wrong thing.** It can say "the app comes after the registry" but not "the app's *push* comes after the registry, and the app's Terraform doesn't care." That distinction is what makes an image push a first-class node, and a node is what gives fail-fast and lets other deploy steps join later.

**It makes the deploy order an accident of imports.** Nothing marks those `load()`s as load-bearing for deployment. Inlining a constant to drop an import — an ordinary, apparently safe refactor — silently deletes an edge.

The graph does *over-approximate* rather than mislead: `//infra/cloud/gcp/audit` loads `//infra/cloud/gcp/lb:defs.bzl` for `DEFAULT_404_BUCKET_NAME`, a **string** in a log-sink filter, and so inherits edges to every app. Extra edges only make an order stricter, never incorrect — audit sorting last is untidy, not broken. So this was rejected for expressiveness, not for correctness.

The underlying distinction is real either way: `lb` needs its backends to *exist in GCP*; `audit` needs a *name*. Both look like "imports a constant from a root", and whether the reference is validated is a property of the Google provider, not of Starlark. It was machine-visible once — `terraform_remote_state` encoded exactly this — until Step 4 of [infra-as-starlark](../infra-as-starlark.md) replaced those data sources with `load()` for fail-fast build-time coupling. That trade stands; this ADR just stops relying on an artifact of it.

### Rejected: a degenerate `tf_root` wrapping the push

`tf_root` accepts `docs = []` and produces a backend-only root, so a push node *could* be a root with no resources — keeping the graph homogeneous and removing the plan/apply split below. But `backend_bucket` is required, so each of the six would create a real GCS state object holding an empty config, take an `init`/`apply` round trip on every deploy, and appear as "No changes." in every plan. A "root" that manages nothing but has state and locking reads as a mistake later. `deploy_task` exists instead.

### Rejected: retry to fixpoint

Applying roots in any order and re-running failures until nothing changes would make order irrelevant. It assumes Terraform is idempotent under *partial* failure, which is not reliably true, and it converts a genuine error — bad credentials, a quota, a real bug — into N failures and an N× log with "which pass was this?" to untangle. Not worth it for a DAG this shallow.

### Rejected: a completeness test over the list

A `bazel test` asserting every `tf_root` appears in `TF_ROOTS` would have caught this exact bug and cost twenty lines. It keeps the list, and with it the synchronization problem in a smaller form — it catches a missing *entry* but not a wrong *position*.

## Prerequisite: examples had to be made unable to reach prod

Defaulting `deploy` to True is only safe because of a change made in the same commit, and the ordering matters.

The four `tf_root` targets under `devtools/bifrost/modules/examples/` each declared `backend_bucket = "senku-prod-terraform-state"` — the real prod state bucket — alongside a live `google_provider`, a Cloud SQL instance and a Secret Manager secret. Each carried a comment reading *"never planned, never applied, just analyzed."* Nothing enforced that comment. **`TF_ROOTS` was the only thing keeping those roots out of prod**, which means deleting the list without touching them would have swept a database and a secret into `senku-prod` on the next apply. An accidental guardrail nobody designed as one.

They now carry two independent defenses:

- `deploy = False`, so the orchestrator never selects them; and
- `backend_bucket = "EXAMPLE-NEVER-APPLIED"`, which is **syntactically invalid** for GCS (uppercase). A hand-run `bazel run //…:terraform.apply` dies inside `terraform init` — before authentication, before any provider call, before any resource exists.

The invalid bucket is the load-bearing one: it holds even for someone bypassing `aspect` entirely, and it cannot be defeated by anyone later creating a bucket with a plausible name. A reader who "fixes" it back to the real bucket reintroduces the hazard, which is the main reason this ADR exists.

Each example also gained the `build_test` its comment already implied, so "just analyzed" is verified rather than asserted.

## Consequences

- **Both tasks shell out to `bazel query` before doing anything.** One query for the set, one for the bootstrap roots under `$CI`, one for the roots-vs-tasks split, then one per node to find its dependents — about 18 today, all but the first scoped to a set of a dozen known targets, ~4s total. A failure returns `None` and both tasks exit non-zero rather than treating "we don't know what to do" as "nothing to do".
- **`aspect plan` and `aspect apply` no longer walk the same graph.** Plan filters to `tf-kind=root`, because there is no plan for pushing an image. Apply walks everything.
- **Cycles are detected and named.** `_topo_sort` reports the nodes it could not place and exits non-zero, rather than silently dropping them.
- **Images are pushed for every deployable root on every full apply**, including unchanged ones — the same total work as before, since each root's `pre_apply` pushed anyway, just reordered to the front.
- **Pushes stay in `pre_apply` as well as being nodes.** Removing them would make a bare `bazel run //apps/A:terraform.apply` deploy a digest that may not exist in the registry: the digest is substituted at build time, and the push is what makes it real. The in-apply push is a near-no-op re-push during an orchestrated run.
- **Using `image_push` directly instead of `registry_push` silently drops the edge.** The failure is loud — the push fails against a missing repository — but it is a convention you have to know.
- **Explicitly-named targets bypass every filter**, so `aspect plan //infra/cloud/gcp/ci:terraform` still works in CI, and a `deploy = False` example can still be planned by name for debugging.
- **`tf_root` is a macro, so its arguments are erased at analysis time.** Tags on the public target are the only channel `bazel query` can still see, which is why the metadata is expressed as tags rather than as a rule attribute.
