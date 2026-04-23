# `service_cloudrun` Terraform module (PoC)

A module for a Cloud Run web service. Parallel to [`service_kubernetes`](../service_kubernetes/README.md), but targeting the managed serverless control plane instead of GKE.

> **Status:** proof of concept.

## Architecture — different from Kubernetes on purpose

Cloud Run is not Kubernetes, and a lot of the K8s module's shape doesn't apply:

| Concern | Kubernetes module | Cloud Run module |
|---|---|---|
| Rollout controller | Flagger / Argo Rollouts (external) | Cloud Run native (revisions + traffic allocation) |
| Server-Side Apply | Yes (`kubernetes_manifest`) | N/A — this is the google provider calling GCP APIs |
| HPA / VPA / PDB | Yes | No — Cloud Run's native scaling subsumes them |
| Secret materialisation | `ephemeral + data_wo` + typed Secret carve-out | Native `secret_key_ref` — Cloud Run reads SM at container startup |
| Pod security context | Hardened at the container level | N/A — every Cloud Run instance runs sandboxed (gVisor or Linux cgroups) |
| Namespaces | Required | None — project + region scopes the service |

Terraform owns everything for Cloud Run — there's no Flagger, no HPA, no `computed_fields` coordination. `terraform apply` with a new `var.image` creates a new revision; the default traffic policy (`TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST`, 100%) sends traffic immediately.

## Resource contract

`resources = { cpu, memory }` maps directly to Cloud Run's `limits`:

- `cpu` in cores (float, e.g. `0.5`) → `"0.5"`
- `memory` in MiB (int, e.g. `512`) → `"512Mi"`

Cloud Run has no request/limit split. The "no CPU limit for web services" rule doesn't apply because each Cloud Run instance is isolated — there's no co-tenant on the same node to throttle.

## Secrets

`secret_env = map({ project, secret, version })` — same shape as the K8s module. Resolution differs: Cloud Run calls Secret Manager at container start and injects the value into the process environment. Nothing is materialised as a K8s Secret; nothing enters Terraform state.

Version must be an explicit integer. `"latest"` is rejected — deploys are immutable, rotation is an explicit version bump that produces a new Cloud Run revision.

## Image pinning

`image` must be digest-pinned (`…@sha256:…`), validated.

## Scaling

`scaling = { min, max }` — Cloud Run allows `min = 0` (scale-to-zero), unlike HPA. The module's validation accepts that.

`concurrency` sets max concurrent requests per instance. This is the native Cloud Run primitive and has no K8s equivalent.

## Traffic / rollouts

The module sets a single traffic block: `TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST` at 100%. That means every apply of a new image takes full traffic immediately. Canary traffic splits (e.g. 10% to new revision, 90% to prior) require revision names that only exist post-apply, so they're not exposed as a module input. Callers who want canaries either:

1. Use Cloud Deploy / Cloud Deploy for Cloud Run (separate control plane, like Flagger for Cloud Run).
2. Override the `google_cloud_run_v2_service.traffic` resource directly outside the module, pinning to named revisions.

## Inputs

See [`variables.tf`](./variables.tf). Notable ones:

- `public` — when `true`, grants `roles/run.invoker` to `allUsers`. Default `false` (private by construction).
- `ingress` — `INGRESS_TRAFFIC_ALL`, `INGRESS_TRAFFIC_INTERNAL_ONLY`, or `INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER`.
- `cpu_idle` — whether CPU is throttled outside requests. `true` (default) is cheaper; `false` keeps CPU allocated always.
- `startup_cpu_boost` — free cold-start speedup, default on.
- `execution_environment` — `EXECUTION_ENVIRONMENT_GEN2` default (Linux cgroups); `GEN1` for legacy gVisor.

## Custom domains

`custom_domains = ["api.example.com", ...]` creates a `google_cloud_run_domain_mapping` per entry. Cloud Run provisions a managed TLS cert once DNS resolves to the returned records.

Caveats inherited from the feature (pick a load balancer instead if any of these hurt):

- Not available in every region. `europe-west1`, `us-central1`, `asia-northeast1`, and a few others are supported; `europe-west4`, `us-east4`, and others are not. See Google's docs for the current list.
- The domain must be verified in the project's Search Console before `terraform apply` succeeds.
- No CDN, no Cloud Armor, no multi-region fan-out, no sharing TLS across services.

Output `custom_domain_dns_records` exposes the `{ type, name, rrdata }` tuples to create in your DNS provider.

## Outputs

- `service_account_email` — runtime GSA; grant IAM on other resources.
- `service_name`, `service_uri`, `service_id` — for DNS wiring, Cloud Scheduler targets, load balancer backends.
- `custom_domain_dns_records` — DNS records to create per domain mapping.

## Verification

```bash
bazel test //devtools/bifrost/terraform/modules/service_cloudrun:lint
bazel run  //devtools/bifrost/terraform/modules/service_cloudrun:validate
```

Lint is `terraform fmt -check -recursive`; semantic validation needs provider downloads and runs locally only.
