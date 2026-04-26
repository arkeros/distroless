# bifrost modules

Opinionated Starlark macros that compose the typed resource constructors
in `//devtools/build/tools/tf/resources:{gcp,k8s}.bzl` into bifrost-shaped
deploys. Drop the returned struct into `tf_root(docs=...)`; nothing else
to wire.

| File | Macros | Use case |
|---|---|---|
| [`cloudrun.bzl`](./cloudrun.bzl) | `service_cloudrun`, `cronjob_cloudrun` | GCP-managed (Cloud Run v2) services and jobs. |
| [`kubernetes.bzl`](./kubernetes.bzl) | `service_kubernetes`, `cronjob_kubernetes` | Self-hosted on a GKE cluster. SSA-applied via `kubernetes_manifest`. |

## What these macros do

Each macro returns a single struct with:

- `.tf` — the merged JSON body for every resource the macro emits, ready
  to drop into a `tf_root(docs=...)` list.
- `.addr` — the bare Terraform address of the "primary" resource (the
  Cloud Run service / Job, or the K8s Deployment / CronJob), so callers
  can `depends_on = [foo.addr]` from siblings.
- `.service_account_email` — the runtime GSA email, ready to slot into a
  `member = "serviceAccount:..."` IAM grant on a downstream resource.
- Cloud Run service: `.uri`, `.id`, `.name`, `.location` (interpolation refs).

## Resource policy

| Variant | CPU | Memory |
|---|---|---|
| K8s web (`service_kubernetes`) | `requests.cpu` only — no limit (avoid CFS throttling) | `requests == limits` |
| K8s batch (`cronjob_kubernetes`) | `requests == limits` | `requests == limits` |
| Cloud Run (any) | `limits.cpu` (Cloud Run shape) | `limits.memory` |

The web rule does not apply to batch — a runaway batch job with no CPU
limit can starve co-tenant web pods on the node.

## Secrets

Cloud Run macros reference Secret Manager natively via
`env.value_source.secret_key_ref`. The Cloud Run control plane resolves
the secret at apply time; the value never enters Terraform state.

K8s macros materialize secret_env as one ephemeral
`google_secret_manager_secret_version` per entry, fed into a
content-hashed `kubernetes_secret_v1.data_wo`. The Secret's name is
`<workload_name>-env-<hash>`; any change to `secret_env` produces a new
Secret object and a new ReplicaSet. Rollback = revert the inputs; the
hash names back to the prior value.

`secret_env[*].version` must be an explicit numeric string;
`"latest"` is rejected by the typed Cloud Run job/service constructors.

## Examples

Each example is a self-contained `tf_root` smoke target — never planned,
never applied, just analyzed at `bazel build` time so a regression in
any macro fails analysis instead of slipping into a real root.

- [`examples/service_cloudrun`](./examples/service_cloudrun/BUILD) — public Cloud Run service.
- [`examples/cronjob_cloudrun`](./examples/cronjob_cloudrun/BUILD) — Cloud Run Job + Cloud Scheduler trigger.
- [`examples/cronjob_kubernetes`](./examples/cronjob_kubernetes/BUILD) — K8s CronJob with `secret_env`.
- [`examples/service_kubernetes`](./examples/service_kubernetes/BUILD) — K8s service composed with a Cloud SQL DB,
  a Secret Manager secret, a migration Job (sibling `depends_on`), and
  an IAM grant on the runtime GSA. Demonstrates cross-resource composition.
