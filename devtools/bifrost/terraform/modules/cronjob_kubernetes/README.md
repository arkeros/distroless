# `cronjob_kubernetes` Terraform module (PoC)

A module for a Kubernetes CronJob with the same architectural shape as the `service` module: Terraform provisions the infrastructure shell via SSA, External Secrets Operator resolves secrets at runtime.

> **Status:** proof of concept. See [`service_kubernetes/README.md`](../service_kubernetes/README.md) for the broader migration note.

## What it creates

- `google_service_account` + `google_service_account_iam_member` — runtime GSA + Workload Identity binding.
- `kubernetes_manifest` / `ServiceAccount` — annotated for Workload Identity.
- `kubernetes_secret_v1` (optional) — typed-resource carve-out for `ephemeral + data_wo`; see [`service_kubernetes/README.md`](../service_kubernetes/README.md#server-side-apply-with-one-carve-out) for the rationale.
- `kubernetes_manifest` / `CronJob` — under SSA, `image` computed so push controllers can own it.

## The Google resource standard (batch variant)

CronJobs set **both** CPU and memory `requests == limits`. The web-serving "requests only on CPU" rule does not apply to batch:

> A misbehaving batch job with no CPU limit can starve co-tenant web pods on the node.

`resources = { cpu = 0.25, memory = 256 }` → `requests.cpu = limits.cpu = "250m"`, `requests.memory = limits.memory = "256Mi"`.

## SSA + immutability

Same model as `service`:

- SSA via `kubernetes_manifest` + `field_manager { name = "terraform" }`.
- `image` in `computed_fields` so push controllers own it on subsequent applies.
- Secrets via `ephemeral + kubernetes_secret_v1.data_wo` (typed carve-out); content-hashed Secret name.
- `image` digest-pinned; `secret_env[*].version` explicit integer (`"latest"` rejected).

## Inputs / outputs / verification

See [`variables.tf`](./variables.tf), [`outputs.tf`](./outputs.tf), and the `service` README for the verification workflow — the pattern is identical.

```bash
bazel test //devtools/bifrost/terraform/modules/cronjob_kubernetes:lint
bazel run  //devtools/bifrost/terraform/modules/cronjob_kubernetes:validate
```
