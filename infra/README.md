# infra

Plain Terraform for everything durable this repo depends on. Applied out of
band, by hand — none of it changes when a new revision ships, so it is
deliberately not wired into CI.

State lives in `gs://senku-prod-terraform-state` under prefix `infra`.

## What lives here

| Resource | Why here and not in the deploy |
| --- | --- |
| Artifact Registry API + `containers` repo | The registry images are pushed to; exists before any deploy. |
| `svc-registry` service account | Runtime identity the Cloud Run services run as. |
| `allUsers` → `run.invoker`, per region | Durable policy. The LB's Serverless NEG calls without an OIDC identity, so the services must accept anonymous invokes; the ingress annotation in the Knative manifest is what keeps them off the open internet. |
| WIF pool, provider, and the CI service accounts | The identities GitHub Actions deploys and caches as. |
| `senku-prod-bazel-cache` bucket + lifecycle | Bazel's remote cache. Durable, and its retention policy is not something a build should be able to change. |

The Cloud Run services themselves are **not** managed here. They are deployed
from [`//oci/cmd/registry:service.yaml`](../oci/cmd/registry/service.yaml) by
the `push-gar` and `deploy` jobs in `.github/workflows/ci.yaml`.

Regions come from
[`//oci/cmd/registry:regions.json`](../oci/cmd/registry/regions.json) — the same
file the deploy matrix is generated from, so the invoker bindings and the
deploy targets cannot drift apart.

## Apply order

The invoker bindings need their service to exist. A brand-new region therefore
deploys first and applies second:

```sh
# 1. add the region to oci/cmd/registry/regions.json, merge, let CI deploy it
# 2. then:
cd infra && terraform apply
```

Every subsequent apply is a no-op. This only bites when adding a region.

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
after validating `prod`'s branch and reviewer rules, so that gate lives in the
identity layer rather than in workflow YAML any committer can edit. The cache
binding is deliberately wider — a cache only `main` can reach is worth very
little — which is why it hangs off an account that can do nothing else. Pull
requests opened from a fork get no OIDC token at all, so cache writers are
exactly the people who can already push a branch here.

After `apply`, confirm the workflow's inputs match:

```sh
terraform output github_workload_identity_provider
terraform output github_service_account
terraform output github_cache_service_account
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
