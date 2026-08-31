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

An invoker binding names a Cloud Run service, and Terraform cannot
`depends_on` a service another system creates — so whether ordering matters
depends on who creates it.

`web`'s services are created here, as shells whose template the deploy then
replaces. Its bindings reference a resource Terraform owns, so the ordering is
an ordinary graph edge and adding a region is one apply.

`registry`'s are not. They predate that arrangement and exist only because the
deploy made them, so a brand-new registry region deploys first and applies
second:

```sh
# 1. add the region to oci/cmd/registry/regions.json, merge, let CI deploy it
# 2. then:
cd infra && terraform apply
```

Every subsequent apply is a no-op. Bringing `registry` in line means adopting
the live services with `import` blocks, which deserves someone watching the
plan run against production — worth doing, not urgent, since the services
already exist and this only bites when a region is added.

## WIF

The `github` pool in this project belongs to `arkeros/senku`; this repo has its
own `github-distroless` pool so that repo's infra cannot widen or revoke access
here. The provider's `attribute_condition` pins `assertion.repository`, so no
other repository can mint a credential in this pool at all.

Inside that gate are two accounts, because the two things CI does need very
different reach:

| Account | Bound to | Can |
| --- | --- | --- |
| `github-actions-distroless` (`github.tf`) | `attribute.environment/prod` | Push to Artifact Registry, deploy Cloud Run, act as `svc-registry` |
| `github-actions-cache` (`cache.tf`) | `attribute.repository` | Read and write the Bazel cache bucket, and nothing else |

The deploy binding is the narrow one: GitHub issues an `environment` claim only
after validating `prod`'s deployment branch policy, which names `main` and
nothing else, so that gate lives in the identity layer rather than in workflow
YAML any committer can edit. The cache binding is deliberately wider — a cache
only `main` can reach is worth very little — which is why it hangs off an
account that can do nothing else. Pull requests opened from a fork get no OIDC
token at all, so cache writers are exactly the people who can already push a
branch here.

After `apply`, confirm the workflow's inputs match:

```sh
terraform output github_workload_identity_provider
terraform output github_service_account
terraform output github_cache_service_account
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
| `main-checks` | `Test` must pass on a branch current with `main`; linear history; no force-push; no deletion | **nobody**, admins included |
| `main-review` | pull request with 1 approval, threads resolved | repository admins |

Folding these into one would mean the bypass that keeps a solo maintainer able
to merge also switched off the status check — and the status check is the
reason the deploy can run unattended. The review requirement is there for when
this repo has more than one maintainer; the check is there for the deploy.

The deploy's own gate is unchanged and does not depend on either ruleset: a job
declaring `environment: prod` on any branch but `main` never starts, so no
token carrying `environment: prod` is minted and the deploy service account
stays unreachable.

## Checking the gate

`repo.tf` adopts the pre-existing `prod` environment through an `import` block,
so a rebuilt state file adopts it rather than colliding with it. The classic
branch protection that used to guard `main` was deleted when the rulesets
replaced it — they cover the same ground more strictly, since classic
protection had `enforce_admins = false` while `main-checks` has no bypass at
all. `main` reports `protected: true` on the strength of the rulesets alone.

To confirm the gate is what this README claims:

```sh
# reviewers gone, branch policy custom and main-only
gh api repos/arkeros/distroless/environments/prod \
  --jq '{rules: [.protection_rules[].type], policy: .deployment_branch_policy}'

# the rules actually in force on main, rulesets merged
gh api repos/arkeros/distroless/rules/branches/main --jq '[.[].type]'

# and that nothing can bypass the status check. The list endpoint omits
# bypass_actors, so this resolves the id and reads the ruleset itself.
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
