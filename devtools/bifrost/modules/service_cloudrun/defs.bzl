"""Starlark form of the bifrost `service_cloudrun` module.

The HCL module in the same directory is the terraform-only entry point;
this `.bzl` is the in-repo Bazel-monorepo equivalent. Two surfaces, same
logical thing: a Cloud Run v2 service with an opinionated default shape
and an optional public-invoker IAM binding.

In-repo callers should prefer this macro — the resource graph lives in
Bazel and composes with `tf_root`. External terraform-only consumers
keep using the HCL module.
"""

load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "google_cloud_run_v2_service",
    "google_cloud_run_v2_service_iam_member",
)

def cloud_run_service(
        name,
        project,
        region,
        image,
        service_account_email,
        service_name = None,
        args = (),
        resources = None,
        scaling = None,
        probes = None,
        env = None,
        secret_env = None,
        ingress = "INGRESS_TRAFFIC_ALL",
        public = False,
        port = 8080,
        cpu_idle = True,
        startup_cpu_boost = True,
        execution_environment = "EXECUTION_ENVIRONMENT_GEN2",
        timeout_seconds = 300,
        concurrency = 80,
        labels = None,
        deletion_protection = False):
    """Cloud Run v2 service plus optional public-invoker IAM binding.

    Returns one struct whose `.tf` body holds both resources (the IAM member
    only when `public=True`); the struct's attribute refs point at the
    service (uri, id, name, location).

    Args:
        name: Terraform resource block key. Must be a valid Terraform
            identifier (letters, digits, `_`). For multi-region fan-out,
            give each region its own unique `name` (e.g. `registry_us_central1`)
            so Terraform addresses don't collide.
        service_name: Cloud Run service `name` attribute. Defaults to `name`.
            Cloud Run names are scoped by (project, location), so multiple
            regions can share `service_name = "registry"` even when their
            `name` differs.
        project: GCP project.
        region: GCP region (Cloud Run v2 service `location`).
        image: Container image, digest-pinned (`@sha256:...`).
        service_account_email: Runtime GSA email. Required — internal SA
            creation is not supported by this macro (declare a
            `service_account(...)` next to the service if you need one).
        args: Container args list.
        resources: Optional `{cpu: number, memory: int (MiB)}`.
        scaling: Optional `{min: int, max: int}`.
        probes: Optional `{startup_path: str, liveness_path: str}`.
        env: Optional `{name: value}` for plain env vars.
        secret_env: Optional `{name: {secret: str, version: str}}` for Secret
            Manager-backed env. Version must be a numeric string.
        ingress: Cloud Run ingress policy.
        public: When True, add IAM binding granting allUsers run.invoker.
        port: Container port (default 8080).
        cpu_idle: Whether CPU is throttled outside requests.
        startup_cpu_boost: Extra CPU during container startup.
        execution_environment: GEN1 vs GEN2.
        timeout_seconds: Request timeout.
        concurrency: Max concurrent requests per instance.
        labels: Extra labels (merged with `{app: <service_name>}`).
        deletion_protection: Cloud Run delete-protection flag. Default False
            because in-repo services are reproducible from code.
    """

    service_name = service_name or name

    final_labels = {"app": service_name}
    if labels:
        final_labels = dict(final_labels)
        final_labels.update(labels)

    container = {
        "image": image,
        "args": list(args),
        "ports": [{"container_port": port}],
    }

    if resources != None:
        container["resources"] = [{
            "limits": {
                "cpu": str(resources["cpu"]),
                "memory": "{}Mi".format(resources["memory"]),
            },
            "cpu_idle": cpu_idle,
            "startup_cpu_boost": startup_cpu_boost,
        }]

    env_blocks = []
    if env:
        for k, v in env.items():
            env_blocks.append({"name": k, "value": v})
    if secret_env:
        for k, v in secret_env.items():
            env_blocks.append({
                "name": k,
                "value_source": [{"secret_key_ref": [{
                    "secret": v["secret"],
                    "version": v["version"],
                }]}],
            })
    if env_blocks:
        container["env"] = env_blocks

    if probes:
        if probes.get("startup_path"):
            container["startup_probe"] = [{
                "http_get": [{"path": probes["startup_path"], "port": port}],
            }]
        if probes.get("liveness_path"):
            container["liveness_probe"] = [{
                "http_get": [{"path": probes["liveness_path"], "port": port}],
            }]

    template = {
        "service_account": service_account_email,
        "execution_environment": execution_environment,
        "max_instance_request_concurrency": concurrency,
        "timeout": "{}s".format(timeout_seconds),
        "containers": [container],
    }
    if scaling != None:
        template["scaling"] = [{
            "min_instance_count": scaling["min"],
            "max_instance_count": scaling["max"],
        }]

    service = google_cloud_run_v2_service(
        name = name,
        project = project,
        location = region,
        service_name = service_name,
        ingress = ingress,
        labels = final_labels,
        deletion_protection = deletion_protection,
        template = template,
        traffic = [{
            "type": "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST",
            "percent": 100,
        }],
    )

    if not public:
        return service

    iam = google_cloud_run_v2_service_iam_member(
        name = "{}_public".format(name),
        project = project,
        location = service.location,
        service_name = service.name,
        role = "roles/run.invoker",
        member = "allUsers",
    )

    # Combine: one struct with both resources in `.tf`, ref attrs from the
    # service (the "primary" resource of the composed pair).
    combined = dict(service.tf["resource"])
    for k, v in iam.tf["resource"].items():
        combined[k] = v

    return struct(
        tf = {"resource": combined},
        addr = service.addr,
        uri = service.uri,
        id = service.id,
        name = service.name,
        location = service.location,
    )
