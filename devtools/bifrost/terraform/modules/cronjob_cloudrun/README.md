# `cronjob_cloudrun` Starlark module (PoC)

Cloud Run Job triggered by Cloud Scheduler, provisioned via the google provider. This module does not touch Kubernetes — the Cloud Run control plane is the reconciler.

The implementation is the [`cronjob_cloudrun`](./defs.bzl) Starlark macro: callers `load(...)` it from a `tf_root`-using BUILD file and pass flat kwargs.

> **Status:** proof of concept.

## What it creates

- `google_service_account` (runtime) — identity the Cloud Run Job runs as.
- `google_service_account` (scheduler) — identity Cloud Scheduler uses to invoke the Job.
- `google_project_iam_member` — `roles/run.invoker` on the scheduler SA.
- `google_cloud_run_v2_job` — the Job definition.
- `google_cloud_scheduler_job` — HTTP trigger posting to the Job's `:run` endpoint.

## The Google resource standard

Cloud Run exposes a single `resources.limits` block covering both CPU and memory; there is no request/limit split. The module maps `resources = { cpu, memory }` directly to those limits.

## Secrets

Cloud Run natively references Secret Manager via `env.value_source.secret_key_ref`. No K8s Secret, no ESO, no content hashing — the Cloud Run control plane handles resolution. `"latest"` remains rejected; version is explicit.

## SSA

Not applicable — Cloud Run isn't Kubernetes. This module uses typed `google_*` resources exclusively.

## Inputs / outputs

See the `cronjob_cloudrun` macro in [`defs.bzl`](./defs.bzl) for argument shapes and the struct fields it returns. A worked usage example lives at [`examples/basic_starlark`](./examples/basic_starlark).
