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

LB_BACKEND = {
    "service_name": "registry",
    # The root that creates the Cloud Run service named above. The LB turns
    # this into a deploy edge, so a backend is always running before anything
    # routes to it.
    "root": "//oci/cmd/registry:terraform",
    "regions": REGIONS,
    "paths": ["/v2/*"],
}
