# infra

Plain Terraform for everything durable this repo depends on. Applied out of
band, by hand — none of it changes when a new revision ships, so it is
deliberately not wired into CI.

Two providers: `google` for the project, and `github` for the handful of repo
settings the deploy's security model actually rests on. The GitHub provider
falls back to `gh auth token`, so applying this needs no PAT beyond the `gh`
login the operator already has.

State lives in `gs://senku-prod-terraform-state` under prefix `infra`.

## What lives here

| Resource | Why here and not in the deploy |
| --- | --- |
| Artifact Registry API + `containers` repo | The registry images are pushed to; exists before any deploy. |
| `svc-registry` service account | Runtime identity the Cloud Run services run as. |
| `allUsers` → `run.invoker`, per region | Durable policy. The LB's Serverless NEG calls without an OIDC identity, so the services must accept anonymous invokes; the ingress annotation in the Knative manifest is what keeps them off the open internet. |
| WIF pool, provider, and the CI service accounts | The identities GitHub Actions deploys and caches as. |
| `senku-prod-bazel-cache` bucket + lifecycle | Bazel's remote cache. Durable, and its retention policy is not something a build should be able to change. |
| `prod` environment + `main` rulesets (`repo.tf`) | GitHub validates the environment's branch policy before minting the OIDC token, so it is part of the identity gate — not a settings page. The rulesets are what make `main` deployable without a human. |

The Cloud Run services themselves are **not** managed here. They are deployed
from [`//oci/cmd/registry:service.yaml`](../oci/cmd/registry/service.yaml) by
the `push-gar` and `deploy` jobs in `.github/workflows/ci.yaml`.

Regions come from
[`//oci/cmd/registry:regions.json`](../oci/cmd/registry/regions.json) — the same
file the deploy matrix is generated from, so the invoker bindings and the
deploy targets cannot drift apart.

## Apply order

None any more, and the reason is worth knowing because it used to bite.

An invoker binding names a Cloud Run service, and Terraform cannot
`depends_on` a service another system creates. While the deploy alone made the
services, a brand-new service or region needed an apply, a deploy, then a
second apply — the first failing with a 404 on a service that did not exist
yet.

Both services' *shells* are Terraform's now, so the bindings reference a
resource in the graph and adding a region is one apply. CI still owns every
revision: `ignore_changes` on the template means an apply never reverts a
deploy, and a deploy never fights an apply.

The first apply after this change adopts the three existing `registry`
services. Expect the plan to read **3 to import, 0 to add, 0 to change, 0 to
destroy**. Anything else — a change, and above all a replacement — means a
service-level field drifted from what `main.tf` declares, and wants
reconciling before the apply rather than after: these three serve
`distroless.io`.

## WIF

The `github` pool in this project belongs to `arkeros/senku`; this repo has its
own `github-distroless` pool so that repo's infra cannot widen or revoke access
here. The provider's `attribute_condition` pins `assertion.repository`, so no
other repository can mint a credential in this pool at all.

Inside that gate are three accounts, because the things CI does need
different reach:

| Account | Bound to | Can |
| --- | --- | --- |
| `github-actions-distroless` (`github.tf`) | `attribute.environment/prod` | Push to Artifact Registry, deploy Cloud Run, act as `svc-registry` |
| `github-actions-cache` (`cache.tf`) | `attribute.environment/prod` | Read and write the Bazel cache bucket, and nothing else |
| `github-actions-cache-ro` (`cache.tf`) | `attribute.repository` | Read the Bazel cache bucket |

The two `prod` bindings are the narrow ones: GitHub issues an `environment`
claim only after validating `prod`'s deployment branch policy, which names
`main` and nothing else, so that gate lives in the identity layer rather than
in workflow YAML any committer can edit. Cache *writes* sit behind it because
the cache is on the path to a signed image: the mirror's provenance says which
run of `main` produced a digest, not what that run read, so a branch that could
write an action result `main` later trusts could get the platform to vouch for
bytes it never built (ADR 0014). Pull requests hold the reader, which is bound
to the repository — any branch can mint it, forks get no OIDC token at all —
and can poison nothing.

After `apply`, confirm the workflow's inputs match:

```sh
terraform output github_workload_identity_provider
terraform output github_service_account
terraform output github_cache_service_account
terraform output github_cache_readonly_service_account
```

## Why the deploy has no manual approval

`prod` used to carry a required-reviewer rule, so every push to `main` waited
on a click. It was removed, because it protected nothing: the sole reviewer was
also the only person who could push to `main`, `prevent_self_review` was off,
and admins could bypass. The same person opened the gate they were standing at.

What the reviewer prompt was standing in for is now enforced directly. `main`
carries two rulesets (`repo.tf`), split because GitHub applies bypass per
ruleset rather than per rule:

| Ruleset | Rules | Bypass |
| --- | --- | --- |
| `main-checks` | `Test` (pr.yaml) must pass on a branch current with `main`; linear history; no force-push; no deletion | repository admins |
| `main-review` | pull request with 1 approval, threads resolved | repository admins |

Both are bypassable by admins today, because a solo maintainer needs a way to
land an emergency fix that cannot wait for `Test`. They stay separate so that
`main-checks` can lose its bypass the day a second maintainer exists without
the review rule following. A bypass is an explicit, audited act in GitHub's
log; it is not the silent path a disabled ruleset would be.

The deploy's own gate is unchanged and does not depend on either ruleset: a job
declaring `environment: prod` on any branch but `main` never starts, so no
token carrying `environment: prod` is minted and the deploy service account
stays unreachable.

## Checking the gate

`repo.tf` adopts the pre-existing `prod` environment through an `import` block,
so a rebuilt state file adopts it rather than colliding with it. The classic
branch protection that used to guard `main` was deleted when the rulesets
replaced it — they cover the same ground, and unlike classic protection an
admin bypass is recorded per event. `main` reports `protected: true` on the
strength of the rulesets alone.

To confirm the gate is what this README claims:

```sh
# reviewers gone, branch policy custom and main-only
gh api repos/arkeros/distroless/environments/prod \
  --jq '{rules: [.protection_rules[].type], policy: .deployment_branch_policy}'

# the rules actually in force on main, rulesets merged
gh api repos/arkeros/distroless/rules/branches/main --jq '[.[].type]'

# and who can bypass the status check (repository admins, actor_id 5). The
# list endpoint omits bypass_actors, so this resolves the id and reads the
# ruleset itself.
gh api repos/arkeros/distroless/rulesets \
  --jq '.[] | select(.name == "main-checks") | .id' \
  | xargs -I{} gh api repos/arkeros/distroless/rulesets/{} --jq '.bypass_actors'
```

## Bazel remote cache

`cache.tf` owns the bucket the `gcs` config in [`//.bazelrc`](../.bazelrc)
points at. The two have to agree on the name, and `terraform output
bazel_remote_cache_url` prints what the bazelrc should say.

Cache only — there is no remote execution, so every action still runs on the
machine that invoked Bazel. Entries expire after 30 days, which bounds growth
without anyone having to prune; a still-warm entry that ages out costs one
rebuild. To use it locally:

```sh
gcloud auth application-default login
bazel build --config=gcs //...
```

## Provider lockfile

`.terraform.lock.hcl` is not committed yet. Generate it with real `terraform`
(not OpenTofu — the hashes are registry-specific):

```sh
cd infra
terraform providers lock \
    -platform=darwin_arm64 -platform=linux_amd64 -platform=linux_arm64
```
