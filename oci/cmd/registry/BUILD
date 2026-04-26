load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_img//img:push.bzl", "image_push")
load(
    "//devtools/bifrost/modules:cloudrun.bzl",
    "service_cloudrun",
)
load("//devtools/build/tools/tf:defs.bzl", "IMAGE_URI", "output", "tf_root")
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
load(
    ":defs.bzl",
    "LB_BACKEND",
    "PROJECT",
    "REGIONS",
)

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

UPSTREAM = "ghcr.io"

REPOSITORY_PREFIX = "arkeros/senku"

sa = service_account(
    name = "registry",
    account_id = "svc-registry",
    display_name = "Runtime identity for registry (shared across all regions)",
    project = PROJECT,
)

# Cloud Run service per region. Resources are inlined directly via the
# `service_cloudrun` Starlark macro — no Terraform `module` block, no
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
    service_cloudrun(
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
        output(
            "lb_backends",
            description = "LB backends exposed for external (terraform-only) consumers. In-repo callers (//infra/cloud/gcp/lb) load `LB_BACKEND` from `:defs.bzl` directly — same content, no `terraform_remote_state` round-trip.",
            value = {"registry": LB_BACKEND},
        ),
        output(
            "service_account_email",
            description = "Runtime GSA shared across every regional Cloud Run service. Grant downstream IAM (e.g. pull credentials, Secret Manager) here once — every region picks it up.",
            value = sa.email,
        ),
        output(
            "service_uris",
            description = "Cloud Run `*.run.app` URL per region. Not reachable directly because of `ingress = INTERNAL_LOAD_BALANCER`; exposed for `gcloud` smoke tests and debugging.",
            value = SERVICE_URIS,
        ),
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

bzl_library(
    name = "defs",
    srcs = ["defs.bzl"],
    visibility = ["//visibility:public"],
    deps = [],  # keep (consumed via load() in //infra/cloud/gcp/lb:defs.bzl — gazelle's bzl_library reverse-dep detection only reads BUILD files, not other .bzl, so without this the rule gets regenerated and visibility stripped)
)
