load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_img//img:push.bzl", "image_push")
load(
    "//devtools/bifrost/terraform/modules/service_cloudrun:defs.bzl",
    "cloud_run_service",
)
load("//devtools/build/tools/tf:defs.bzl", "IMAGE_URI", "tf_root")
load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "google_provider",
    "service_account",
)
load(
    "//infra/cloud/gcp/gar:defs.bzl",
    "GAR_REGISTRY",
    "GAR_REPOSITORY_PREFIX",
)
load(
    "//oci:config.bzl",
    "OCI_REGISTRY",
    "OCI_REPOSITORY_PREFIX",
)
load("//oci:go_image.bzl", "go_images_all_arch")
load("//oci/distroless/common:variables.bzl", "COMPRESSION", "DEBUG_MODE")

package(default_visibility = [
    "//oci/cmd/registry:__subpackages__",
])

go_library(
    name = "registry_lib",
    srcs = ["main.go"],
    importpath = "github.com/arkeros/senku/oci/cmd/registry",
    visibility = ["//visibility:private"],
    deps = [
        "//base/flag/stringslice",
        "//oci/pkg/proxy",
    ],
)

go_binary(
    name = "registry",
    embed = [":registry_lib"],
    visibility = ["//visibility:public"],
)

go_images_all_arch(
    name = "image",
    binary = ":registry",
    distro = "debian13",
    static = True,
)

# Two push destinations per mode: GHCR for the public `distroless.io` mirror,
# GAR for the Cloud Run deploy path (Cloud Run can't pull from GHCR directly).
# Same image, same digest, two hosts — pushed independently.
[
    image_push(
        name = "image" + mode + "_push_" + dest_suffix,
        image = ":image" + mode + "_nonroot",
        registry = registry,
        repository = repository_prefix + "/registry",
        stamp = "enabled",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
    )
    for mode in DEBUG_MODE
    for dest_suffix, registry, repository_prefix in [
        ("ghcr", OCI_REGISTRY, OCI_REPOSITORY_PREFIX),
        ("gar", GAR_REGISTRY, GAR_REPOSITORY_PREFIX),
    ]
]

# ---------------------------------------------------------------------------
# Terraform root for the registry deploy.
#
# One Cloud Run service per region, all sharing the same logical name
# (`registry`) and the same runtime GSA. Region fan-out is for latency, not
# identity — the services are replicas of the same workload, so they share
# one SA (one row in audit logs / IAM bindings, not three). Cloud Run service
# names are region-scoped, so `registry` in every region does not collide.
# Fronting them behind a single anycast IP is the job of `infra/cloud/gcp/lb`.
# ---------------------------------------------------------------------------

PROJECT = "senku-prod"

REGIONS = [
    "us-central1",
    "europe-west3",
    "asia-northeast1",
]

UPSTREAM = "ghcr.io"

REPOSITORY_PREFIX = "arkeros/senku"

sa = service_account(
    name = "registry",
    account_id = "svc-registry",
    display_name = "Runtime identity for registry (shared across all regions)",
    project = PROJECT,
)

# Cloud Run service per region. Resources are inlined directly via the
# `cloud_run_service` Starlark macro — no Terraform `module` block, no
# bifrost HCL files copied into the workdir. Each call produces both the
# `google_cloud_run_v2_service.registry_<region>` and (because public=True)
# its `google_cloud_run_v2_service_iam_member.registry_<region>_public`.
#
# Ingress locked to LB-only. Org policy on senku-prod forbids
# `INGRESS_TRAFFIC_ALL`, and architecturally the registry sits behind the
# shared external HTTPS LB anyway. `public = True` stays because the LB's
# Serverless NEG calls without injecting an OIDC identity, so the services
# must accept anonymous invokes — the ingress filter is what keeps the
# service off the open internet.
services = [
    cloud_run_service(
        name = "registry_" + region.replace("-", "_"),
        args = [
            "--upstream=" + UPSTREAM,
            "--repository-prefix=" + REPOSITORY_PREFIX,
        ],
        # Sentinel substituted with the digest URI at Bazel build time
        # (see `tf_root.image_push` below). No `var.image` round-trip.
        image = IMAGE_URI,
        ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER",
        probes = {
            "startup_path": "/v2/",
            "liveness_path": "/v2/",
        },
        project = PROJECT,
        public = True,
        region = region,
        resources = {
            "cpu": 1,
            "memory": 512,
        },
        scaling = {
            "min": 0,
            "max": 3,
        },
        # Share one identity across regions: one principal in audit logs /
        # IAM bindings, not three.
        service_account_email = sa.email,
        service_name = "registry",
    )
    for region in REGIONS
]

# Output value: an HCL object literal mapping region → service uri,
# expanded at Starlark evaluation. Each entry references the inlined
# `google_cloud_run_v2_service.registry_<region>` resource directly.
SERVICE_URIS = "${ {" + ", ".join([
    '"%s" = google_cloud_run_v2_service.registry_%s.uri' % (
        region,
        region.replace("-", "_"),
    )
    for region in REGIONS
]) + "} }"

tf_root(
    name = "terraform",
    docs = [
        google_provider(project = PROJECT),
        sa,
    ] + services + [
        {"output": {"lb_backends": {
            # `sorted(REGIONS)` mimics the previous `set(string)` semantics:
            # the old `var.regions` was a set, which Terraform serializes in
            # alphabetical order; preserve that so the LB root's plan shows
            # no diff after this migration.
            "value": {"registry": {
                "service_name": "registry",
                "regions": sorted(REGIONS),
                "paths": ["/v2/*"],
            }},
            "description": "LB backends exposed for `infra/cloud/gcp/lb` to read via `terraform_remote_state`. The single `registry` backend fans out across every deploy region — one NEG per region, all attached to one backend service, so the LB picks the closest healthy region per request.",
        }}},
        {"output": {"service_account_email": {
            "value": sa.email,
            "description": "Runtime GSA shared across every regional Cloud Run service. Grant downstream IAM (e.g. pull credentials, Secret Manager) here once — every region picks it up.",
        }}},
        {"output": {"service_uris": {
            "value": SERVICE_URIS,
            "description": "Cloud Run `*.run.app` URL per region. Not reachable directly because of `ingress = INTERNAL_LOAD_BALANCER`; exposed for `gcloud` smoke tests and debugging.",
        }}},
    ],
    image_push = ":image_push_gar",
    required_providers = {
        "google": {
            "source": "hashicorp/google",
            "version": "~> 7.0",
        },
    },
    visibility = ["//visibility:public"],
)
