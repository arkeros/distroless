"""Identity constants for the registry deploy.

The LB root reads `LB_BACKEND` directly (Starlark, build-time) rather than
through a `terraform_remote_state` data source — same content, one less
runtime indirection. The Terraform `lb_backends` output stays for external
consumers that aren't in this monorepo.
"""

PROJECT = "senku-prod"

REGIONS = [
    "us-central1",
    "europe-west3",
    "asia-northeast1",
]

# `sorted` mirrors the previous `set(string)` semantics: Terraform serialised
# `var.regions` (a set) in alphabetical order, and the LB still depends on
# that ordering for its plan to be a no-op after this migration.
LB_BACKEND = {
    "service_name": "registry",
    "regions": sorted(REGIONS),
    "paths": ["/v2/*"],
}
