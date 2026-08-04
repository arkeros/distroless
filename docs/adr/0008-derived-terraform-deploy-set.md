# The Terraform deploy set is derived from the build graph, not listed

`aspect plan` / `aspect apply` discover which Terraform roots to walk by querying the build graph for tags that each `tf_root` publishes about itself — membership (`tf-deploy`), credential tier (`tf-bootstrap`) and deploy order (`tf-tier=<n>`). The hand-maintained `TF_ROOTS` list in `.aspect/stdlib.axl` is deleted. Joining the deploy DAG is now the same act as declaring the root, so there is nothing to register and nothing to forget.

## Why

`TF_ROOTS` was an ordered list of ten entries, edited roughly once per new app, carrying three facts the orchestrator needed. It drifted the first time it was load-bearing under time pressure: `//apps/spaghetti-duel:terraform` was added to the repo without a corresponding entry. Nothing caught it — the app built, tested, reviewed, merged and passed CI. The load balancer root *was* in the list, so it applied happily and created a certificate, a host rule, a backend service and a serverless NEG naming a Cloud Run service that nothing would ever create.

That failure is quiet in a specific and expensive way: **a serverless NEG whose backing Cloud Run service does not exist serves a bare `404` rather than failing.** No error, no unhealthy backend, no signal in any dashboard — just a 404 with no backend headers, indistinguishable from a missing asset. Diagnosis cost an afternoon and went down two wrong paths (a mid-issuance TLS certificate, then a suspected routing bug) before arriving at "the service was never created."

A list that must be edited in lockstep with another file is a synchronization problem, and this repo already had evidence it loses that game: `.aspect/README.md` documented a `BOOTSTRAP_ROOTS` constant that had been refactored away into `struct(bootstrap = True)` some time earlier, and nobody noticed.

## What each fact rides on

| Fact | Where it lives | Cost of adding a root |
| --- | --- | --- |
| Membership | `deploy = True`, the `tf_root` default | none |
| Bootstrap tier | `bootstrap = True` | two call sites, both pre-existing |
| Order vs. the registry | `tf_root_with_image` sets `deploy_tier` | none — inherited from the macro |
| Order vs. the LB | `deploy_tier = LB_DEPLOY_TIER` | one call site, one constant |

`deploy` defaults to **True** deliberately. The two mistakes are not symmetric: a root that silently fails to deploy is a 404 that surfaces hours later in a browser, while a root that deploys when it shouldn't fails loudly on the next apply. Defaulting to in makes the silent failure structurally impossible and leaves only the loud one — but see *Prerequisite* below, because that reasoning does not hold on its own.

## Order is tiers, not edges

Roots apply in ascending `deploy_tier`; within a tier, order is alphabetical and must not matter. Tier 0 is everything self-sufficient (the registry, the bootstrap roots), tier 1 is every root that pushes an image, tier 2 is the load balancer.

Tiers rather than per-root `deploy_after` edges because tiers are **conservative in the direction that matters**. A new root defaults to tier 0 and therefore applies *before* the load balancer at tier 2 — which is the constraint that actually bites. A missed edge in a `deploy_after` scheme fails the other way: silently late, which is the bug we are fixing.

`tf_root_with_image` assigns tier 1 by construction: it prepends an `image_push` to `pre_apply`, so the registry must exist before terraform runs. A new app therefore inherits a correct position from the macro it was already using, without its author knowing tiers exist.

### Rejected: infer the order from the `load()` graph

Tempting, because the graph already looks like the answer — `buildfiles(deps(//infra/cloud/gcp/lb:terraform))` really does name every app. It is unsound. `//infra/cloud/gcp/audit` loads `//infra/cloud/gcp/lb:defs.bzl` to read `DEFAULT_404_BUCKET_NAME`, a **string** interpolated into a log-sink filter, and thereby inherits a build edge to all five apps and the LB. Topologically sorting that graph relocates a bootstrap-tier root to last place because of a string import.

The distinction the graph cannot express: `lb` needs its backends to *exist in GCP*; `audit` needs a *name*. Both are "imports a constant from a root." Whether the reference is validated is a property of the Google provider's API semantics, not of Starlark, and no query can recover it. It was knowable once — `terraform_remote_state` encoded exactly this — but Step 4 of [infra-as-starlark](../infra-as-starlark.md) deliberately replaced those data sources with `load()` for fail-fast build-time coupling, trading a machine-visible deploy dependency for a build-time constant. That trade stands; this ADR just stops pretending the dependency is still inferable.

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

- **`aspect plan` / `aspect apply` now shell out to `bazel query` before doing anything.** Four queries: one for the set, one for the bootstrap roots under `$CI`, then one per occupied tier scoped to the set already found. A query failure returns `None` and both tasks exit non-zero rather than treating "we don't know what to do" as "nothing to do".
- **Tiers are single-digit.** Tags are matched by substring, so `tf-tier=1` would otherwise also select `tf-tier=10`; the macro rejects anything outside `0..9`.
- **Explicitly-named roots bypass every filter**, so `aspect plan //infra/cloud/gcp/ci:terraform` still works in CI, and a `deploy = False` example can still be planned by name for debugging.
- **A root with no image push that the LB routes to would land at tier 0**, applying before the LB — correct, if pessimistically early. A backend needing tier ≥ 2 would be a deliberate act and would need its own tier.
- **`tf_root` is a macro, so its arguments are erased at analysis time.** Tags on the public `filegroup` are the only channel `bazel query` can still see, which is why the metadata is expressed as tags rather than as a rule attribute.
