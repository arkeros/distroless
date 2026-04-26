# `service_kubernetes` Starlark module (PoC)

A module for a Kubernetes web service, shaped for a **TF-as-provisioner + Flagger-as-pusher** architecture.

The implementation is the [`service_kubernetes`](./defs.bzl) Starlark macro; callers `load(...)` it from a `tf_root`-using BUILD file and pass flat kwargs.

> **Status:** proof of concept. No caller yet. The Go-generated `*.generated.tf` path remains the source of truth for production workloads.

## Architecture (the boundary)

| Concern | Owner | Tool |
|---|---|---|
| GCP identity (GSA, Workload Identity binding) | Terraform | `google_*` |
| Deployment spec (image, env, resources, probes) | Terraform | `kubernetes_manifest` via **SSA** |
| Service, HPA, PDB, VPA (recommender) | Terraform | `kubernetes_manifest` via **SSA** |
| Secret materialisation | Terraform (typed carve-out — see below) | `kubernetes_secret_v1 + ephemeral + data_wo` |
| `spec.replicas` on the Deployment | HPA (steady state) + Flagger (briefly during canary) | — |
| Canary rollout, traffic split, rollback on SLO breach | Flagger | sibling resources it owns |
| Cross-resource wiring (`env.DB_HOST = module.db.hostname`, `depends_on`) | Terraform | HCL |

Flagger **does not write back** to your Deployment. It watches it for spec changes (new image, new env), and on a change it creates/updates a sibling `-primary` Deployment plus shadow services, splits traffic, and promotes by copying your Deployment's spec into `-primary`. So `terraform apply` with a new `var.image` is what triggers a canary.

## Server-Side Apply (with one carve-out)

Every Kubernetes object is created via `kubernetes_manifest` with `field_manager { name = "terraform" }`. The Deployment yields ownership of `spec.replicas` via `computed_fields` — HPA owns it long-term, Flagger touches it briefly during canaries. The container `image` is **not** in `computed_fields`: Flagger doesn't write to the target Deployment, so Terraform owns image normally.

**The one typed-resource exception is `kubernetes_secret_v1`.** Reason: `kubernetes_manifest` doesn't expose write-only on its dynamic `manifest` attribute, and ephemeral values can only flow into write-only destinations. The `hashicorp/kubernetes` provider hasn't shipped a `manifest_wo` sidecar or per-path write-only annotations yet. Since the Secret is single-writer (only this module touches it), losing SSA coordination on that one object costs nothing. When the provider gains write-only manifest support, the resource flips with a one-line change.

**Cost of SSA:** `terraform plan` opens a connection to the apiserver to dry-run each manifest against the live schema. Plans fail on an unreachable cluster. This is the tradeoff you get for explicit multi-writer coordination.

## PushOps / MPM-style immutability

- `image` must be digest-pinned (`…@sha256:…`); validated.
- `secret_env[*].version` must be an explicit integer; `"latest"` is rejected.
- The K8s Secret name is content-hashed: `<name>-env-<hash>`, where the hash is a stable Starlark `hash()` over a deterministic JSON serialization of `secret_env`. Any change to `secret_env` produces a new Secret object and a new ReplicaSet. Rollback = revert the inputs; the hash names back to its prior value.

## Secrets: ephemeral + write-only

Values flow `GCP Secret Manager → ephemeral "google_secret_manager_secret_version" → kubernetes_secret_v1.data_wo`. The `ephemeral` block reads at apply time only; the value never enters Terraform state. The `data_wo` attribute writes into K8s but is never read back — also never in state. Rotation = bump `version` on the input; the hash changes; the Secret's name changes; a new Secret is created; a new ReplicaSet rolls out.

No cluster prerequisites beyond a reachable apiserver.

## The Google resource standard (web-serving variant)

- `requests.cpu` only — no `limits.cpu` (avoid CFS throttling on latency-sensitive code).
- `requests.memory == limits.memory` — memory is not burstable.

`resources = { cpu = 0.25, memory = 512 }` → `requests.cpu = "250m"`, `requests.memory = limits.memory = "512Mi"`.

CronJobs use the batch variant (both CPU and memory request==limit); see `cronjob_kubernetes/README.md`.

## Composition

The macro returns a struct that drops directly into `tf_root(docs=...)`. Compose by passing `env` values that interpolate other resources (e.g. `env = {"DB_HOST": db.private_ip_address}` where `db` is the struct from `google_sql_database_instance(...)`), and emit any sibling resources (migration Jobs, IAM members for the runtime GSA) as separate entries in the same `docs` list.

## Inputs

See the macro signature in [`defs.bzl`](./defs.bzl). Key ones:

- `image` — digest-pinned; validated.
- `resources = { cpu = float, memory = int }` — cores and MiB.
- `secret_env = map({ project, secret, version })` — SM references; `"latest"` rejected.
- `autoscaling = { min, max, target_cpu_utilization }` — `min >= 1` enforced (no silent clamp).
- `probes = { startup_path, liveness_path, readiness_path }` — all optional; readiness is how a pod signals "not ready for traffic right now" during graceful shutdown or slow warmups.
- `vpa_enabled` — default `true`. Emits a VPA in recommender-only mode (`updateMode = "Off"`): observes actual usage, surfaces recommendations on the VPA's status, never auto-mutates pods. Requires the VPA CRD in the cluster; set `false` if unavailable.

A `PodDisruptionBudget` is emitted automatically when `autoscaling.min >= 2` (`maxUnavailable = 1`). On a single-replica service a PDB is pointless; on `min == max == 1` it blocks all voluntary disruption.

## Outputs

- `service_account_email` — the runtime GSA; grant IAM to this on DBs, buckets, secrets.
- `kubernetes_service_account_name`, `deployment_name`, `service_name`.

## Verification

The smoke target at [`examples/basic_starlark`](./examples/basic_starlark) builds the macro into JSON at Bazel analysis time, so a typo or missing kwarg fails `bazel build` before it can slip into a real root.

True semantic correctness — does SSA ownership split work as designed, does `data_wo` behave as expected under drift — is only verifiable by applying against a real cluster.
