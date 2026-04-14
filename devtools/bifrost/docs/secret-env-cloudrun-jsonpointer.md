# Cloud Run: JSON Pointer support for secretEnv

## Problem

Cloud Run's native Secret Manager integration (`valueFrom.secretKeyRef`) gives you the raw secret value — the whole thing. There's no field extraction. On Kubernetes, `resolve-secrets` handles JSON Pointer extraction, spread, and transforms before the Secret reaches the cluster. Cloud Run has no equivalent pipeline.

This means `secretEnv` on Cloud Run rejects fragments (`#/host`) and spread (`...`) with a clear error, while Kubernetes supports them fully.

## When this matters

When the secret is a JSON blob you don't control — e.g., Aiven gives you a single JSON config with host, port, user, password. You need individual fields as env vars.

When you control the secret format, the answer is simpler: store each value as a separate secret in Secret Manager.

## Options

### Option 1: Deploy-time resolution (recommended)

Make `resolve-secrets` work on Cloud Run manifests. Instead of native `valueFrom.secretKeyRef`, bifrost generates plain `env.value` placeholders with the secret URI. A deploy-time step fetches the secret, applies JSON Pointer extraction, and injects the resolved value into the YAML before `gcloud run deploy`.

**Pros:**
- Same pipeline as Kubernetes — `resolve-secrets` handles everything
- Supports all transforms: JSON Pointer, spread, base64 decode
- No app changes, no image changes

**Cons:**
- Secret values end up as plaintext in the deploy manifest (visible in CI logs, Cloud Run revision YAML)
- Requires `resolve-secrets` in the Cloud Run deploy pipeline (currently only used for K8s)

**Implementation sketch:**
1. When `secretEnv` contains fragments or spread, bifrost generates `env.value` entries with the raw URI instead of `valueFrom.secretKeyRef`
2. `resolve-secrets` is extended to process Cloud Run Knative YAML (currently only processes K8s Secret objects)
3. The deploy pipeline runs `resolve-secrets` on the Cloud Run YAML before `gcloud run deploy`

### Option 2: App-level resolution

The app reads the whole JSON secret and parses it itself. This is the most common pattern at Google — apps own their config parsing.

**Pros:**
- Simplest — no tooling changes
- No security tradeoff — secret stays in Secret Manager

**Cons:**
- Every app reinvents the same JSON parsing
- Leaks infrastructure concerns into application code
- Doesn't work with third-party images

### Option 3: Wrapper entrypoint

A shell script that uses `jq` to extract fields before exec'ing the real binary.

**Pros:**
- Works without app changes

**Cons:**
- Requires `jq` in the image
- Fragile, hard to debug
- Doesn't compose well with distroless/scratch images

## Recommendation

**Short term:** Document the limitation. Users who need JSON Pointer on Cloud Run should store each value as a separate secret.

**Medium term:** Implement Option 1 when there's real demand. The security tradeoff (plaintext values in deploy manifest) is acceptable for most internal services but should be documented clearly.

## Current status

- [x] `secretEnv` on Cloud Run: plain GCP URIs with native `valueFrom.secretKeyRef`
- [x] Cross-project secrets via `run.googleapis.com/secrets` annotation
- [x] Clear error messages for unsupported features (spread, fragments, transforms)
- [ ] JSON Pointer / spread support on Cloud Run (this doc)
