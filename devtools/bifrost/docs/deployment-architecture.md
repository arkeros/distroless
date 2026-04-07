# Deployment Architecture: Bifrost + Flux + OCI

Inspired by Google's internal deployment system (Rapid + MPM + Borg config), built entirely on open tools.

## Architecture overview

```
Bifrost (build time)     →  OCI Registry (artifact store)  →  Flux (runtime)

render manifests              store versioned bundles           pull + apply in order
structure into phases         verify signatures                dependsOn for sequencing
generate Flux resources       immutable tags                   health-gated promotion
push as OCI artifact                                           automatic rollback
```

## Google → Open source mapping

| Google | Open source equivalent |
|---|---|
| MPM package (versioned binary) | OCI image (container registry) |
| Borg config (versioned config) | OCI artifact (YAML manifests in registry) |
| Release (binary + config + strategy) | `OCIRepository` + `Kustomization` |
| Rapid (sequencing) | Flux kustomize-controller |
| Rapid (canary) | Flagger |
| Rapid (rollback) | Flagger + `OCIRepository` pinning |
| Borgmon (health signals) | Prometheus metrics |

## How it works

### 1. Bifrost renders and pushes an OCI artifact

Bazel or CI builds the manifests and pushes them as an immutable, versioned, signed OCI artifact:

```bash
bifrost k8s render -e env.yaml -f service.yaml > manifests/

flux push artifact oci://registry.io/apps/registry:v2 \
    --path=manifests/ \
    --source=git@github.com:arkeros/senku \
    --revision=v2.0.0
```

### 2. Flux pulls it with OCIRepository

source-controller polls the registry, fetches new versions, and verifies signatures:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: registry
spec:
  url: oci://registry.io/apps/registry
  ref:
    tag: v2
  interval: 1m
```

### 3. Phased deployment with dependsOn

Flux's `Kustomization` already supports ordered phases with health gating:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: registry-migrate
spec:
  sourceRef:
    kind: OCIRepository
    name: registry
  path: ./phases/migrate
  wait: true
  timeout: 5m

---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: registry-deploy
spec:
  dependsOn:
    - name: registry-migrate
  sourceRef:
    kind: OCIRepository
    name: registry
  path: ./phases/deploy
  wait: true
```

kustomize-controller won't start `registry-deploy` until `registry-migrate` is healthy.

### 4. Canary deployments with Flagger

Flagger watches a Deployment, creates a canary clone, shifts traffic gradually, and promotes or rolls back based on Prometheus metrics:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: registry
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: registry
  service:
    port: 8080
  analysis:
    interval: 1m
    threshold: 5          # max failed checks before rollback
    maxWeight: 50         # max traffic % to canary
    stepWeight: 10        # increment per interval
    metrics:
      - name: request-success-rate
        thresholdRange:
          min: 99
      - name: request-duration
        thresholdRange:
          max: 500
```

On a new image push, Flagger progressively shifts traffic:

```
 0% canary → 10% → 20% → 30% → 40% → 50% → promote to 100%
                              ↓
                     metrics degrade?
                              ↓
                         rollback to 0%
```

Combined with phased deployment, the full flow becomes:

```
migrate (Job, wait for complete)
  → deploy (Deployment + Canary, Flagger manages traffic shift)
    → promote (automatic if metrics pass)
    → rollback (automatic if metrics fail)
```

## OCI artifact structure

Bifrost renders everything — the K8s manifests and the Flux orchestration manifests:

```
oci://registry.io/apps/registry:v2
├── phases/
│   ├── migrate/
│   │   └── job.yaml
│   └── deploy/
│       ├── serviceaccount.yaml
│       ├── deployment.yaml
│       ├── canary.yaml
│       ├── hpa.yaml
│       └── service.yaml
└── flux/
    ├── source.yaml          # OCIRepository
    ├── migrate.yaml         # Kustomization phase 1
    └── deploy.yaml          # Kustomization phase 2
```

## Bazel integration

```python
bifrost_service(
    name = "registry",
    image_push = "//services/registry:push",
    canary = canary(
        step_weight = 10,
        max_weight = 50,
        interval = "1m",
        metrics = [
            metric("request-success-rate", min = 99),
            metric("request-duration", max = 500),
        ],
    ),
    phases = [
        phase("migrate", job = "//services/registry/migrations:push"),
        phase("deploy"),
    ],
)
```

Outputs an OCI artifact with phased manifests, Canary resource, and Flux Kustomizations wired together.

## Why no custom controller

Flux already provides the needed primitives:

| Need | Flux feature |
|---|---|
| Versioned artifact delivery | `OCIRepository` |
| Signature verification | cosign integration |
| Ordered phases | `Kustomization.dependsOn` |
| Wait for readiness | `Kustomization.wait` + `timeout` |
| Rollback | Flagger + `OCIRepository.ref.tag` pinning |
| Health monitoring | built-in health checks |
| Canary / progressive delivery | Flagger |
| Traffic shifting | Flagger + Istio VirtualService |

No custom CRD, no operator to maintain. Bifrost stays a build-time tool; Flux handles runtime orchestration.

## Runtime assumptions

- **Istio** service mesh is present in the cluster. Flagger uses Istio VirtualService for traffic shifting between canary and primary workloads.
