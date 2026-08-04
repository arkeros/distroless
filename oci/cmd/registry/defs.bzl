"""Identity constants for the registry deploy.

The LB root reads `LB_BACKEND` directly (Starlark, build-time) rather than
through a `terraform_remote_state` data source — same content, one less
runtime indirection. The Terraform `lb_backends` output stays for external
consumers that aren't in this monorepo.
"""

load("@terraform.bzl", "ref")

PROJECT = "senku-prod"

REGIONS = [
    "us-central1",
    "europe-west3",
    "asia-northeast1",
]

SERVICE_NAME = "registry"

_LB_BACKEND_FIELDS = {
    "regions": REGIONS,
    "paths": ["/v2/*"],
}

# For //infra/cloud/gcp/lb, which loads this: the service is named by
# reference, so the LB's deploy edge and the value it routes to are the same
# token — they cannot come to disagree. Published by this package's `tf_root`
# as an export.
LB_BACKEND = dict(
    _LB_BACKEND_FIELDS,
    service_name = ref("//oci/cmd/registry:terraform", "service_name"),
)

# The same fact with the name inlined, for the `lb_backends` Terraform output
# this root publishes. A reference only resolves inside the document of the
# root that makes it, and this root cannot reference itself in any case — it
# is the source of the value, not a consumer of it.
LB_BACKEND_TF = dict(_LB_BACKEND_FIELDS, service_name = SERVICE_NAME)
