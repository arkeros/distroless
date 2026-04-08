# Bifrost Roadmap

## Phase 1: Core completeness

- [x] Environment variables (`env` and `envFrom` fields in spec)
- [x] PodDisruptionBudget for K8s services (derived from `minReplicas`)
- [ ] One-shot Job workload type (migrations, backfills)

## Phase 3: Networking and routing

- [ ] Ingress / Gateway API support for K8s
- [ ] Cloud Run domain mappings
- [ ] Terraform DNS records for custom domains
- [ ] NetworkPolicy defaults (deny-all + allow declared ports)

## Phase 4: Observability

- [ ] Prometheus scrape annotations on pods
- [ ] Optional ServiceMonitor CRD generation
- [ ] Structured logging configuration
- [ ] Tracing sidecar support

## Phase 5: Multi-environment

- [ ] `bifrost diff` command — show what changes across environments
- [ ] `bifrost plan` — dry-run render against multiple environments
- [ ] Promotion workflows (dev -> staging -> prod)

## Phase 6: Advanced platform features

- [ ] Cloud Run VPC Connector configuration
- [ ] Cloud Run execution environment (gen1/gen2)
- [ ] K8s custom metrics for HPA (beyond CPU)
- [ ] Terraform state import helpers for existing infrastructure
- [ ] Multi-region deployment support
