# Bifrost

`bifrost` is a bridge between developer intent and platform-native infrastructure. Like Bifröst in Norse mythology, it connects different worlds: a small workload API on one side, and Knative, Kubernetes, and Terraform artifacts on the other. The goal is to give developers a simpler and more stable authoring surface, improve portability across targets, and make cross-domain integration easier, while still producing native outputs for each platform.

The tools use the familiar `<context> <noun> <verb>` style of CLI interactions. For example, to render a service from a YAML file, you would run:

```bash
bifrost cloudrun render -e ./environment.yaml -f ./service.yaml
bifrost k8s render -e ./environment.yaml -f ./service.yaml
bifrost terraform render -e ./environment.yaml -f ./service.yaml
```

Current outputs:

- `Service`:
  - Cloud Run Knative YAML
  - Kubernetes `ServiceAccount` + `Deployment` + `HorizontalPodAutoscaler` + `Service`
  - Terraform for runtime identity (`google_service_account`, plus GKE Workload Identity binding when `kubernetes` is set)
- `CronJob`:
  - Cloud Run `run.googleapis.com/v1 Job` YAML
  - Kubernetes `ServiceAccount` + `CronJob`
  - Terraform for runtime identity and Cloud Scheduler trigger

The source model lives under [`api/`](./api). The CLI in [`cli/`](./cli) is one consumer of that API.

## Setup

See the repo [Setup section](../README.md) for Bazelisk installation, `direnv`, `bazel run //tools:dev`, and `direnv allow`.

After that, `bifrost` is available from the repo root.

## Quickstart

Create an environment file with shared infrastructure identity:

```yaml
# environment.yaml
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Environment
metadata:
  name: senku-prod
spec:
  gcp:
    projectId: senku-prod
    projectNumber: "874944788122"
    region: europe-west1
  kubernetes:
    namespace: jobs
```

Define a workload:

```yaml
# service.yaml
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Service
metadata:
  name: registry
spec:
  image: registry
  port: 8080
  resources:
    limits:
      cpu: 1000m
      memory: 256Mi
  cloudRun:
    ingress: all
```

Render:

```bash
bifrost cloudrun render -e environment.yaml -f service.yaml
bifrost k8s render -e environment.yaml -f service.yaml
bifrost terraform render -e environment.yaml -f service.yaml
```

`bifrost` accepts either YAML or JSON input for both files.

## Environment

An `Environment` holds shared infrastructure identity that is reused across many workloads:

- `gcp.projectId`, `gcp.projectNumber`, `gcp.region` — GCP project coordinates
- `kubernetes.namespace`, `kubernetes.serviceType` — Kubernetes target config

One environment is shared by all services and cronjobs deployed to the same project and namespace. Per-workload settings like `cloudRun` and `cloudScheduler` stay on the workload.

The `--environment` / `-e` flag is required for all render commands.

## Bazel Macro

The main in-repo integration is [`bifrost_service`](./bifrost.bzl) and [`bifrost_cronjob`](./bifrost.bzl).

They let you define a workload in Starlark and generate:

- `<name>.workload.json`
- `<name>.cloudrun.yaml`
- `<name>.k8s.yaml`
- `<name>.terraform.tf`

The `environment` parameter accepts a Starlark dict:

```starlark
load("//devtools/bifrost:bifrost.bzl", "bifrost_service")

bifrost_service(
    name = "registry",
    image = "registry",
    port = 8080,
    args = [
        "--upstream=ghcr.io",
        "--repository-prefix=arkeros/senku",
    ],
    resources = {
        "requests": {
            "cpu": "250m",
            "memory": "256Mi",
        },
        "limits": {
            "cpu": "1000m",
            "memory": "256Mi",
        },
    },
    autoscaling = {
        "min": 0,
        "max": 3,
    },
    environment = {
        "gcp": {
            "projectId": "senku-prod",
            "projectNumber": "874944788122",
            "region": "europe-west3",
        },
    },
)
```

Alternatively, use `environment_file` to point to a YAML or JSON file:

```starlark
bifrost_service(
    name = "registry",
    environment_file = "//env:senku-prod.yaml",
    # ...
)
```

`environment` (dict) and `environment_file` (label) are mutually exclusive — exactly one is required.

The macro emits JSON because Starlark can serialize JSON safely with the built-in `json` module. `bifrost <target> render` consumes that generated JSON the same way it consumes a hand-written YAML or JSON file.

### Digest-Pinned Images

Instead of a plain string, `image_push` accepts a Bazel label pointing to an `image_push` target from `@rules_img`. At build time, the deploy manifest is read to resolve the image to a fully-qualified digest-pinned reference (e.g. `ghcr.io/arkeros/senku/registry@sha256:abc...`).

```starlark
bifrost_service(
    name = "registry",
    image_push = "//oci/cmd/registry:image_nonroot_push",
    # ...
)
```

`image` and `image_push` are mutually exclusive. Use `image` for plain strings (development, external images), and `image_push` for production builds pinned to a specific digest.

### Checked-In Outputs

The macro also supports checked-in generated files through `checked_in = {...}`.

Example:

```starlark
bifrost_service(
    name = "registry",
    # ...
    checked_in = {
        "terraform": "terraform/registry.generated.tf",
    },
)
```

For each checked-in target, the macro creates a `write_source_file` update target and adds a header like:

```text
# Generated by bifrost.bzl
# To update this file, run:
#   bazel run //oci/deploy:registry_terraform_update
```

That keeps generated files reviewable while still giving Bazel a concrete sync target and diff test.

## Why This Exists

`bifrost` exists because deployment manifests are a poor long-term source of truth for platform intent.

Knative YAML and Kubernetes YAML are already target-specific outputs. Once they become the primary authoring format, shared intent gets duplicated and infrastructure decisions get buried in platform syntax.

The model here is:

1. define workload intent once
2. render target-specific deploy artifacts
3. render only the supporting infrastructure that is safe to derive from that intent

That is closer to a small platform API than to a templating trick.

## Design Decisions

### Environment vs Workload

Configuration is split into two files:

- **Environment** — shared infrastructure identity: GCP project, region, Kubernetes namespace. One environment is reused across many workloads.
- **Workload** — application-specific intent: image, resources, scaling, probes, and platform-specific behavior like `cloudRun.ingress` or `cloudScheduler.retryCount`.

The boundary is: if two workloads in the same project could legitimately need different values, the field belongs on the workload. If it's always the same for a given deployment target, it belongs on the environment.

### Custom Service API

The service API is defined in [`api/`](./api), not embedded in the CLI package.

Why:

- the schema is reusable
- versioning belongs with the API
- the binary is just one renderer

### Shared Intent vs Platform Knobs

Shared fields live once at the top level:

- `image`
- `serviceAccountName`
- `args`
- `port`
- `resources`
- `probes`
- `autoscaling`
- `secretEnv`

Platform-specific fields stay under their platform key:

- `cloudRun.ingress`, `cloudRun.public`
- `cloudScheduler.retryCount`, `cloudScheduler.attemptDeadlineSeconds`

Environment-level fields:

- `gcp.projectId`, `gcp.projectNumber`, `gcp.region`
- `kubernetes.namespace`, `kubernetes.serviceType`

That split keeps source intent stable while letting renderers translate to Cloud Run and Kubernetes naming.

### Secret Environment Variables

`secretEnv` injects secrets from providers (GCP Secret Manager, env vars, files) as container environment variables:

```yaml
spec:
  env:
    APP_ENV: production
  secretEnv:
    API_KEY: gcp:///projects/P/secrets/catalog-api-key/versions/3
    DB_HOST: gcp:///projects/P/secrets/aiven-pg-secret/versions/1#/host
    ...catalog: gcp:///projects/P/secrets/catalog-env/versions/1
```

Values are secret provider URIs from [`platform/kubernetes/secrets`](../../platform/kubernetes/secrets/README.md). URI features work:

- **JSON Pointer** (`#/host`) — extract a field from a JSON secret
- **Spread** (`...` prefix) — expand all top-level keys from a JSON secret into separate env vars
- **Base64 decode** (`?decode=base64`, `?payload=base64`) — ingress/egress decoding

For Kubernetes, `secretEnv` generates a K8s Secret with the URIs as `stringData`, and adds `envFrom.secretRef` to the container. [`resolve-secrets`](../resolve-secrets/README.md) resolves the URIs before the Secret reaches the cluster.

Plain `env` values take precedence over `secretEnv` (K8s `env` overrides `envFrom`).

In Starlark:

```starlark
bifrost_service(
    name = "myapp",
    secret_env = {
        "API_KEY": "gcp:///projects/P/secrets/catalog-api-key/versions/3",
    },
    # ...
)
```

**Platform behavior:**

- **Kubernetes** — generates a K8s Secret with URIs as `stringData` + `envFrom.secretRef`. Supports JSON Pointer, spread, and all transforms (resolved by `resolve-secrets`).
- **Cloud Run** — generates native `valueFrom.secretKeyRef` entries. Only plain `gcp://` URIs are supported (no fragments, spreads, or transforms).

### Explicit Runtime Identity

`serviceAccountName` is optional. If omitted, `bifrost` derives a default Google service account email from `metadata.name` and the environment's `gcp.projectId`.

That gives a good default without silently falling back to broad platform defaults.

Identity is rendered differently by target:

- Cloud Run uses the GSA directly
- Kubernetes uses a KSA named after the service and annotates it for GKE Workload Identity
- Terraform creates the GSA, and when `kubernetes` is set, also creates the `roles/iam.workloadIdentityUser` binding

### Shared Autoscaling

`autoscaling` is a shared block because the intent is the same across targets:

```yaml
autoscaling:
  min: 0
  max: 3
```

Current mapping:

- Cloud Run:
  - `autoscaling.min` -> `autoscaling.knative.dev/minScale`
  - `autoscaling.max` -> `autoscaling.knative.dev/maxScale`
  - `autoscaling.concurrency` -> `containerConcurrency`
- Kubernetes:
  - `autoscaling.min` -> HPA `minReplicas`
  - `autoscaling.max` -> HPA `maxReplicas`
  - `autoscaling.targetCPUUtilization` -> HPA CPU target

One real platform difference remains:

- Cloud Run can use `min = 0`
- Kubernetes HPA is clamped to `minReplicas >= 1`

That mismatch is handled in the renderer instead of duplicating the field in the source model.

### Kubernetes-Shaped Resources

`spec.resources` uses Kubernetes `core/v1.ResourceRequirements` directly.

That coupling is intentional in this repo:

- the repository already depends heavily on Kubernetes APIs
- the shape is familiar
- it reduces conversion code in the renderers

## What Bifrost Generates

### Cloud Run

`bifrost cloudrun render` emits a Knative `serving.knative.dev/v1 Service` for Cloud Run deploys.

It carries:

- ingress
- execution environment
- scaling annotations
- runtime service account

### Kubernetes

`bifrost k8s render` emits:

- `ServiceAccount`
- `Deployment`
- `HorizontalPodAutoscaler`
- `Service`

It does not set fixed `Deployment.spec.replicas`; horizontal scaling is owned by the HPA.

### Terraform

`bifrost terraform render` is intentionally narrow. It emits supporting infrastructure, not the workload itself.

For **Services**:

- `google_service_account` — runtime identity
- `google_service_account_iam_member` — GKE Workload Identity binding (only when `kubernetes` is set in the environment)

For **CronJobs**, it additionally emits:

- `google_service_account` — Cloud Scheduler invoker identity
- `google_project_iam_member` — `roles/run.invoker` for the scheduler SA
- `google_cloud_scheduler_job` — the cron trigger pointing at the Cloud Run Job

It does not emit Cloud Run service/job resources, DNS, load balancers, or custom domains.

That split is deliberate: deploy artifacts own the workload, Terraform owns the sustaining identity and scheduling.

## Validation and Defaults

Validation focuses on required and security-relevant inputs:

- `metadata.name` is required
- `spec.image` is required
- `spec.port` must be positive
- CPU and memory limits are required
- Environment: `gcp.projectId`, `gcp.projectNumber`, `gcp.region` are required
- `serviceAccountName`, if set, must be a GSA email matching the environment's project

Defaults are used for low-risk operational knobs:

- Cloud Run ingress defaults to `all`
- Cloud Run execution environment defaults to `gen2`
- Kubernetes service type defaults to `ClusterIP` (when `kubernetes` is set)
- Kubernetes namespace defaults to `default` (when `kubernetes` is set)
- autoscaling fills in `concurrency` and CPU target defaults
- requests default to limits when omitted

## Current Scope

`bifrost` currently models two workload kinds:

- `Service`
- `CronJob`

The point of the current design is to prove that one higher-level workload model can generate:

- Cloud Run deploy artifacts
- Kubernetes deploy artifacts
- Terraform supporting infrastructure

If that continues to hold, it is the right abstraction level for adding more workload kinds later.

For `CronJob`, the source model is split into:

- `schedule`
  Cross-platform trigger intent such as cron and time zone.
- `job`
  Cross-platform execution settings such as parallelism, completions, retries, and timeout.
- `cloudScheduler`
  Google-specific scheduler configuration (retry count, attempt deadline) used only for the Cloud Run + Cloud Scheduler render path. Region is shared via the environment's `gcp.region`.
