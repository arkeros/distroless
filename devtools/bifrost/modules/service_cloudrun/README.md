# `service_cloudrun` — Starlark macro

A Cloud Run web service, declared in Starlark and emitted into a `tf_root`'s
generated `.tf.json`. The macro composes `google_cloud_run_v2_service` and
(when `public = True`) `google_cloud_run_v2_service_iam_member` from the
[1:1 wrappers](../../../../../devtools/build/tools/tf/resources/gcp.bzl) into
one logical unit, with bifrost's opinionated defaults baked in.

## Usage

```python
load(
    "//devtools/bifrost/terraform/modules/service_cloudrun:defs.bzl",
    "cloud_run_service",
)

services = [
    cloud_run_service(
        name = "registry_" + region.replace("-", "_"),
        service_name = "registry",
        project = "senku-prod",
        region = region,
        image = IMAGE_URI,                  # sentinel; tf_root splices the digest at build time
        service_account_email = sa.email,
        scaling = {"min": 0, "max": 3},
        resources = {"cpu": 1, "memory": 512},
        probes = {"startup_path": "/v2/", "liveness_path": "/v2/"},
        ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER",
        public = True,
    )
    for region in REGIONS
]
```

See [`oci/cmd/registry/BUILD`](../../../../../oci/cmd/registry/BUILD) for the
canonical end-to-end usage.

## Architecture — different from Kubernetes on purpose

Cloud Run is not Kubernetes, and a lot of the K8s module's shape doesn't apply:

| Concern | Kubernetes module | Cloud Run macro |
|---|---|---|
| Rollout controller | Flagger / Argo Rollouts (external) | Cloud Run native (revisions + traffic allocation) |
| HPA / VPA / PDB | Yes | No — Cloud Run's native scaling subsumes them |
| Secret materialisation | `ephemeral + data_wo` + typed Secret carve-out | Native `secret_key_ref` — Cloud Run reads SM at container startup |
| Pod security context | Hardened at the container level | N/A — every Cloud Run instance runs sandboxed (gVisor or Linux cgroups) |
| Namespaces | Required | None — project + region scopes the service |

Terraform owns everything for Cloud Run — there's no Flagger, no HPA, no
`computed_fields` coordination. `terraform apply` with a new `image` creates a
new revision; the default traffic policy (`TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST`,
100%) sends traffic immediately.

## Notable inputs

- `name` — **Terraform block key**, must be a valid identifier (`a-z`, `0-9`, `_`).
- `service_name` — Cloud Run service `name` field. Defaults to `name`. Cloud Run names are scoped by `(project, location)`, so multi-region fan-out can share `service_name = "foo"` while each region has its own unique block key.
- `image` — digest-pinned URI. Use the `IMAGE_URI` sentinel from `tf_root`'s render layer to splice the build's just-built digest at build time.
- `resources = { cpu, memory }` — CPU in cores (float, e.g. `0.5`), memory in MiB (int). No request/limit split.
- `scaling = { min, max }` — `min = 0` is allowed (scale-to-zero). `concurrency` is a separate kwarg, default 80.
- `probes = { startup_path, liveness_path }` — HTTP GET paths.
- `ingress` — `INGRESS_TRAFFIC_ALL` | `INTERNAL_ONLY` | `INTERNAL_LOAD_BALANCER`.
- `public = True` adds `roles/run.invoker` for `allUsers`. Default `False`.
- `cpu_idle` (default `True`), `startup_cpu_boost` (default `True`),
  `execution_environment` (default `EXECUTION_ENVIRONMENT_GEN2`).
- `env`, `secret_env` — plain and Secret Manager-backed env vars. `secret_env`
  versions must be explicit integers; `"latest"` is rejected.

See `defs.bzl` for the full list and defaults.

## Outputs

The returned struct exposes attribute interpolation strings for downstream
references (e.g. an LB NEG pointing at the service):

- `.uri` — `${google_cloud_run_v2_service.<name>.uri}`
- `.id`, `.name`, `.location` — same shape

Plus `.tf` (the JSON body) and `.addr` (bare address for `depends_on`).

## Why Starlark, not HCL

The HCL form of this module was deleted when the registry root migrated to
`tf_root`. Reasons:

- The HCL module was duplicating what the Starlark macro now expresses, with
  inevitable drift between the two.
- The only in-repo consumer (`oci/cmd/registry`) used the Starlark form.
- The standalone Terraform sample (`infra/cloud/gcp/lb/examples/hello`) was a
  documentation artifact for the HCL flow; with no HCL flow to document, it's
  redundant.

External terraform-only consumers should pin a tag of this repo from before
the deletion, or copy the resource graph into their own root.
