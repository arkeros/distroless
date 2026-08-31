load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_img//img:image.bzl", "image_from_binary")
load("@rules_img//img:load.bzl", "image_load")
load("@rules_img//img:push.bzl", "image_push")
load(
    "@terraform.bzl//:gcp.bzl",
    "google_cloud_run_v2_service",
    "google_cloud_run_v2_service_iam_member",
    "google_provider",
    "service_account",
)
load("@terraform.bzl", "output", "tf_root")
load("//devtools/build/tools/tf:render.bzl", "image_uri", "registry_push")
load("//infra/cloud/gcp/gar:defs.bzl", "REGISTRY")
load(
    "//oci:config.bzl",
    "OCI_REGISTRY",
    "OCI_REPOSITORY_PREFIX",
)
load("//oci:mirror_push.bzl", "mirror_push")
load("//oci:supply_chain.bzl", "image_supply_chain")
load("//oci/distroless/common:variables.bzl", "BUSYBOX_WONTFIX_CVES", "DEBIAN_WONTFIX_CVES", "DEBUG_MODE")
load(
    ":defs.bzl",
    "LB_BACKEND_TF",
    "PROJECT",
    "REGIONS",
    "SERVICE_NAME",
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

[
    image_from_binary(
        name = "image" + mode,
        base = "//oci/distroless/static:static%s_nonroot_debian" % mode,
        binary = ":registry",
        # This particular Go app is a statically linked binary with no runfiles deps.
        include_runfiles = False,
        platforms = [
            "//bazel/platforms:linux_amd64",
            "//bazel/platforms:linux_arm64",
        ],
    )
    for mode in DEBUG_MODE
]

[
    image_supply_chain(
        ignore_cves = DEBIAN_WONTFIX_CVES + (BUSYBOX_WONTFIX_CVES["debian"] if mode == "_debug" else []),
        image = ":image" + mode,
    )
    for mode in DEBUG_MODE
]

[
    image_load(
        name = "image%s_load" % mode,
        image = ":image%s" % mode,
        tag_list = [
            "bazel/%s:%s" % (
                package_name(),
                "image%s" % mode,
            ),
        ],
    )
    for mode in DEBUG_MODE
]

# GHCR — the public `distroless.io` mirror surface. Signed + SLSA-attested
# via mirror_push (ADR 0006). External consumers verify with cosign.
[
    mirror_push(
        name = "image" + mode,
        image = ":image" + mode,
        repository = "registry",
        sbom = ":image" + mode + "_sbom",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
        tags = ["manual"],
    )
    for mode in DEBUG_MODE
]

# GAR — internal Cloud Run deploy substrate. Same digest as the GHCR copy,
# different registry. IAM-trusted, NOT cosign-trusted: out of mirror_push
# scope per ADR 0006. `aspect apply` re-pushes against the same digest via
# the registry root's pre_apply hook.
[
    registry_push(
        name = "image" + mode + "_push_gar",
        # Only the non-debug image is deployed. The debug variant exists for
        # `gcloud run services proxy` sessions and is pushed by hand, so it is
        # built here but stays out of the deploy DAG.
        deploy = not mode,
        image = ":image" + mode,
        registry = REGISTRY,
        repository = "registry",
        tag_list = [
            "debug" if mode else "latest",
            "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}" + mode.replace("_", "-"),
            "{{.STABLE_MONOREPO_SHORT_VERSION}}" + mode.replace("_", "-"),
        ],
    )
    for mode in DEBUG_MODE
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

CATALOG = [
    # TODO: keep sorted and up-to-date
    "bash",
    "nginx",
]

sa = service_account(
    name = "registry",
    account_id = "svc-registry",
    display_name = "Runtime identity for registry (shared across all regions)",
    project = PROJECT,
)

# Cloud Run service per region, as plain Terraform resources. The
# `google_cloud_run_v2_service` / `_iam_member` constructors from
# `@terraform.bzl//:gcp.bzl` are 1:1 wrappers over the provider schema, so the
# blocks below are what lands in `main.tf.json` — no module indirection.
#
# Ingress locked to LB-only. Org policy on senku-prod forbids
# `INGRESS_TRAFFIC_ALL`, and architecturally the registry sits behind the
# shared external HTTPS LB anyway. The `allUsers` invoker binding stays
# because the LB's Serverless NEG calls without injecting an OIDC identity, so
# the services must accept anonymous invokes — the ingress filter is what keeps
# the service off the open internet.

PORT = 8080

services = [
    google_cloud_run_v2_service(
        name = "registry_" + region.replace("-", "_"),
        deletion_protection = False,
        ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER",
        labels = {"app": SERVICE_NAME},
        location = region,
        project = PROJECT,
        service_name = SERVICE_NAME,
        template = {
            "containers": [{
                # A `ref` to the push node's digest export: resolved at build
                # time, and it brings this root's deploy edge on the push along
                # with it. No `var.image` round-trip.
                "image": image_uri(":image_push_gar"),
                "args": [
                    "--upstream=" + UPSTREAM,
                    "--repository-prefix=" + REPOSITORY_PREFIX,
                ] + [
                    "--repo=" + image
                    for image in CATALOG
                ],
                "ports": [{"container_port": PORT}],
                "resources": [{
                    "limits": {
                        "cpu": "1",
                        "memory": "512Mi",
                    },
                    "cpu_idle": True,
                    "startup_cpu_boost": True,
                }],
                "startup_probe": [{"http_get": [{
                    "path": "/v2/",
                    "port": PORT,
                }]}],
                "liveness_probe": [{"http_get": [{
                    "path": "/v2/",
                    "port": PORT,
                }]}],
            }],
            "execution_environment": "EXECUTION_ENVIRONMENT_GEN2",
            "max_instance_request_concurrency": 80,
            "scaling": [{
                "min_instance_count": 0,
                "max_instance_count": 3,
            }],
            # Share one identity across regions: one principal in audit logs /
            # IAM bindings, not three.
            "service_account": sa.email,
            "timeout": "300s",
        },
        traffic = [{
            "type": "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST",
            "percent": 100,
        }],
    )
    for region in REGIONS
]

# `location`/`service_name` are refs into the service blocks above rather than
# literals — that is what gives Terraform the binding → service dependency edge.
public_invokers = [
    google_cloud_run_v2_service_iam_member(
        name = "registry_%s_public" % REGIONS[i].replace("-", "_"),
        location = services[i].location,
        member = "allUsers",
        project = PROJECT,
        role = "roles/run.invoker",
        service_name = services[i].name,
    )
    for i in range(len(REGIONS))
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
    backend_bucket = "senku-prod-terraform-state",
    # Published for other roots to `ref`. The LB names this to build its
    # serverless NEG, and takes its deploy edge from the same reference —
    # so a backend is always running before anything routes to it.
    exports = {"service_name": SERVICE_NAME},
    docs = [
        google_provider(project = PROJECT),
        sa,
    ] + services + public_invokers + [
        output(
            "lb_backends",
            description = "LB backends exposed for external (terraform-only) consumers. In-repo callers (//infra/cloud/gcp/lb) load `LB_BACKEND` from `:defs.bzl` directly — same content, no `terraform_remote_state` round-trip.",
            value = {"registry": LB_BACKEND_TF},
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
    # The digest in `main.tf.json` is baked in at build time; the push is what
    # makes it real. Keeping it here means a bare `bazel run :terraform.apply`
    # is correct on its own, not only under `aspect apply`.
    pre_apply = [":image_push_gar"],
    providers = ["@terraform_providers//:google"],
    visibility = ["//visibility:public"],
)

bzl_library(
    name = "defs",
    srcs = ["defs.bzl"],
    visibility = ["//visibility:public"],
    deps = [],  # keep (consumed via load() in //infra/cloud/gcp/lb:defs.bzl — gazelle's bzl_library reverse-dep detection only reads BUILD files, not other .bzl, so without this the rule gets regenerated and visibility stripped)
)
