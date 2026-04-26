"""Bifrost macros for Cloud Run services and jobs.

`service_cloudrun(...)` — Cloud Run v2 service plus an optional public-invoker
IAM binding. Returns one struct shaped for `tf_root(docs=...)` whose attribute
fields point at the underlying service (uri, id, name, location).

`cronjob_cloudrun(...)` — Cloud Run v2 Job plus a Cloud Scheduler cron trigger,
a runtime GSA, an invoker GSA, and the run.invoker IAM binding that lets the
scheduler call the job.

Both macros build the per-container env list (plain values + Secret Manager
references via `value_source.secret_key_ref`) the same way; that's the only
shared helper.
"""

load("//devtools/build/tools/tf:defs.bzl", "merge_tf")
load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "google_cloud_run_v2_job",
    "google_cloud_run_v2_job_iam_member",
    "google_cloud_run_v2_service",
    "google_cloud_run_v2_service_iam_member",
    "google_cloud_scheduler_job",
    "service_account",
)

_DEFAULT_JOB = {
    "parallelism": 1,
    "completions": 1,
    "max_retries": 3,
    "timeout_seconds": 600,
}

def _env_blocks(env, secret_env):
    """Cloud Run container env: plain values + Secret Manager refs.

    Cloud Run accepts both shapes inline in the same `env` list, distinguished
    by `value` vs `value_source.secret_key_ref`.
    """
    blocks = []
    if env:
        for k, v in env.items():
            blocks.append({"name": k, "value": v})
    if secret_env:
        for k, v in secret_env.items():
            blocks.append({
                "name": k,
                "value_source": [{"secret_key_ref": [{
                    "secret": v["secret"],
                    "version": v["version"],
                }]}],
            })
    return blocks

# ── service_cloudrun ───────────────────────────────────────────────────────

def service_cloudrun(
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

    Returns one struct whose `.tf` body holds the service (and the IAM
    member only when `public=True`); attribute refs point at the service
    (uri, id, name, location).

    Args:
        name: Terraform resource block key. Must be a valid Terraform
            identifier. For multi-region fan-out, give each region a unique
            `name` (e.g. `registry_us_central1`) so addresses don't collide.
        service_name: Cloud Run service `name` attribute. Defaults to `name`.
            Multiple regions can share `service_name = "registry"` even when
            their `name` differs (Cloud Run names are scoped by location).
        project: GCP project.
        region: GCP region (Cloud Run v2 service `location`).
        image: Container image, digest-pinned (`@sha256:...`).
        service_account_email: Runtime GSA email. Required — internal SA
            creation is not supported by this macro.
        args: Container args list.
        resources: Optional `{cpu: number, memory: int (MiB)}`.
        scaling: Optional `{min: int, max: int}`.
        probes: Optional `{startup_path: str, liveness_path: str}`.
        env: Optional `{name: value}` for plain env vars.
        secret_env: Optional `{name: {secret: str, version: str}}` for Secret
            Manager-backed env. Version must be a numeric string.
        ingress: Cloud Run ingress policy.
        public: When True, add an IAM binding granting allUsers run.invoker.
        port: Container port (default 8080).
        cpu_idle: Whether CPU is throttled outside requests.
        startup_cpu_boost: Extra CPU during container startup.
        execution_environment: GEN1 vs GEN2.
        timeout_seconds: Request timeout.
        concurrency: Max concurrent requests per instance.
        labels: Extra labels (merged with `{app: <service_name>}`).
        deletion_protection: Cloud Run delete-protection flag. Default False.
    """
    service_name = service_name or name

    final_labels = {"app": service_name}
    if labels:
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

    blocks = _env_blocks(env, secret_env)
    if blocks:
        container["env"] = blocks

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

    return struct(
        tf = merge_tf(service, iam),
        addr = service.addr,
        uri = service.uri,
        id = service.id,
        name = service.name,
        location = service.location,
    )

# ── cronjob_cloudrun ───────────────────────────────────────────────────────

def cronjob_cloudrun(
        name,
        project,
        region,
        image,
        schedule,
        resources,
        job_name = None,
        args = (),
        env = None,
        secret_env = None,
        job = None,
        cloud_scheduler = None,
        service_account_id = None):
    """Cloud Run v2 Job + Cloud Scheduler trigger + runtime/scheduler GSAs.

    Returns one struct holding five resources: runtime GSA, scheduler GSA,
    the Cloud Run Job, a job-scoped `roles/run.invoker` binding granting
    the scheduler GSA permission to invoke this job, and the Cloud Scheduler
    trigger. Attribute refs on the returned struct point at the Job; the
    runtime/scheduler GSA emails are exposed for downstream IAM grants.

    Args:
        name: Terraform block-key prefix.
        project: GCP project.
        region: GCP region for both the Job and the Scheduler.
        image: Container image, digest-pinned.
        schedule: `{cron: str, time_zone: str}` for the trigger.
        resources: `{cpu: number, memory: int (MiB)}` — both enforced as limits.
        job_name: Cloud Run Job name (kebab-case, GCP rules). Defaults to
            `name`. SA account_ids derive from `job_name`, so callers whose
            `name` is snake_case should pass `job_name` in kebab-case.
        args: Container args list.
        env: Optional `{name: value}` for plain env vars.
        secret_env: Optional `{name: {secret, version}}` for Secret Manager.
        job: Optional `{parallelism, completions, max_retries, timeout_seconds}`.
        cloud_scheduler: Optional `{retry_count, attempt_deadline_seconds}`.
            `retry_count = 0` (default) emits no retry_config.
        service_account_id: Override the runtime GSA `account_id`.
            Defaults to `crj-<job_name>`.
    """
    job_name = job_name or name
    runtime_account_id = service_account_id or "crj-{}".format(job_name)
    scheduler_account_id = "sch-{}".format(job_name)

    job_settings = dict(_DEFAULT_JOB)
    if job:
        job_settings.update(job)

    runtime_sa = service_account(
        name = "{}_runtime".format(name),
        project = project,
        account_id = runtime_account_id,
        display_name = "Runtime identity for cronjob {}".format(job_name),
    )

    scheduler_sa = service_account(
        name = "{}_scheduler".format(name),
        project = project,
        account_id = scheduler_account_id,
        display_name = "Cloud Scheduler invoker for {}".format(job_name),
    )

    container = {
        "image": image,
        "args": list(args),
        "resources": [{"limits": {
            "cpu": str(resources["cpu"]),
            "memory": "{}Mi".format(resources["memory"]),
        }}],
    }
    blocks = _env_blocks(env, secret_env)
    if blocks:
        container["env"] = blocks

    job_resource = google_cloud_run_v2_job(
        name = name,
        project = project,
        location = region,
        job_name = job_name,
        template = {
            "parallelism": job_settings["parallelism"],
            "task_count": job_settings["completions"],
            "template": [{
                "service_account": runtime_sa.email,
                "timeout": "{}s".format(job_settings["timeout_seconds"]),
                "max_retries": job_settings["max_retries"],
                "containers": [container],
            }],
        },
    )

    invoker = google_cloud_run_v2_job_iam_member(
        name = "{}_scheduler_invoker".format(name),
        project = project,
        location = job_resource.location,
        job_name = job_resource.name,
        role = "roles/run.invoker",
        member = "serviceAccount:{}".format(scheduler_sa.email),
    )

    cs = cloud_scheduler or {}
    retry_count = cs.get("retry_count", 0)
    attempt_deadline_seconds = cs.get("attempt_deadline_seconds")

    trigger = google_cloud_scheduler_job(
        name = name,
        project = project,
        region = region,
        scheduler_name = "{}-trigger".format(job_name),
        schedule = schedule["cron"],
        time_zone = schedule["time_zone"],
        http_target = {
            "http_method": "POST",
            "uri": "https://run.googleapis.com/v2/projects/{project}/locations/{region}/jobs/{job_name}:run".format(
                project = project,
                region = region,
                job_name = job_name,
            ),
            "oauth_token": [{"service_account_email": scheduler_sa.email}],
        },
        attempt_deadline = "{}s".format(attempt_deadline_seconds) if attempt_deadline_seconds else None,
        retry_config = {"retry_count": retry_count} if retry_count > 0 else None,
    )

    return struct(
        tf = merge_tf(runtime_sa, scheduler_sa, job_resource, invoker, trigger),
        addr = job_resource.addr,
        id = job_resource.id,
        name = job_resource.name,
        location = job_resource.location,
        service_account_email = runtime_sa.email,
        scheduler_service_account_email = scheduler_sa.email,
    )
