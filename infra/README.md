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
| WIF pool, provider, and the CI service account | The identity GitHub Actions deploys as. |

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
here. Two gates, both in `github.tf`:

- the provider's `attribute_condition` pins `assertion.repository`, so no other
  repository can mint a credential in this pool at all;
- the service account binding is keyed on `attribute.environment/prod`, which
  GitHub only issues after validating the environment's branch and reviewer
  rules.

After `apply`, confirm the workflow's inputs match:

```sh
terraform output github_workload_identity_provider
terraform output github_service_account
```

## Provider lockfile

`.terraform.lock.hcl` is not committed yet. Generate it with real `terraform`
(not OpenTofu — the hashes are registry-specific):

```sh
cd infra
terraform providers lock \
    -platform=darwin_arm64 -platform=linux_amd64 -platform=linux_arm64
```
