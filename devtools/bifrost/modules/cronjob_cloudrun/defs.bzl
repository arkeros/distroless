"""Starlark form of the bifrost `cronjob_cloudrun` module.

The HCL module in the same directory is the terraform-only entry point;
this `.bzl` is the in-repo Bazel-monorepo equivalent. Two surfaces, same
logical thing: a Cloud Run v2 Job triggered by a Cloud Scheduler cron,
with a runtime GSA, an invoker GSA, and the run.invoker IAM binding
that lets the scheduler call the job.

In-repo callers should prefer this macro — the resource graph lives in
Bazel and composes with `tf_root`. External terraform-only consumers
keep using the HCL module.
"""

load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "google_cloud_run_v2_job",
    "google_cloud_scheduler_job",
    "project_iam_member",
    "service_account",
)

_DEFAULT_JOB = {
    "parallelism": 1,
    "completions": 1,
    "max_retries": 3,
    "timeout_seconds": 600,
}

_DEFAULT_SCHEDULER = {
    "retry_count": 0,
    "attempt_deadline_seconds": None,
}

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

    Returns one struct whose `.tf` body holds five resources:
      - `google_service_account.<name>_runtime`
      - `google_service_account.<name>_scheduler`
      - `google_project_iam_member.<name>_scheduler_invoker` (run.invoker)
      - `google_cloud_run_v2_job.<name>`
      - `google_cloud_scheduler_job.<name>`

    Attribute refs on the returned struct point at the Cloud Run Job
    (id, name, location, addr). The runtime and scheduler GSA emails are
    exposed as `service_account_email` and `scheduler_service_account_email`,
    matching the HCL module's outputs.

    Args:
        name: Terraform resource block key prefix. Each generated resource
            uses `name` (or a `<name>_<role>` derivation) as its block key.
        project: GCP project.
        region: GCP region for both the Job and the Scheduler.
        image: Container image, digest-pinned (`@sha256:...`).
        schedule: `{cron: str, time_zone: str}` for the trigger.
        resources: `{cpu: number, memory: int (MiB)}` — both enforced as limits.
        job_name: Cloud Run Job `name` attribute (kebab-case, GCP rules).
            Defaults to `name`. The scheduler trigger's name is
            `<job_name>-trigger`. SA account_ids derive from `job_name`,
            so callers whose `name` is snake_case (TF identifier) should
            pass `job_name` in kebab-case explicitly.
        args: Container args list.
        env: Optional `{name: value}` for plain env vars.
        secret_env: Optional `{name: {secret: str, version: str}}` for Secret
            Manager-backed env. Version must be a numeric string (no "latest").
        job: Optional `{parallelism, completions, max_retries, timeout_seconds}`
            overrides for the Job execution (defaults: 1, 1, 3, 600).
        cloud_scheduler: Optional `{retry_count, attempt_deadline_seconds}`.
            `retry_count = 0` (default) emits no `retry_config` block.
        service_account_id: Override the runtime GSA `account_id`. Defaults
            to `crj-<job_name>`. The scheduler GSA `account_id` is always
            `sch-<job_name>` (it's an internal implementation detail).
    """
    job_name = job_name or name
    runtime_account_id = service_account_id or "crj-{}".format(job_name)
    scheduler_account_id = "sch-{}".format(job_name)

    job_settings = dict(_DEFAULT_JOB)
    if job:
        job_settings.update(job)

    cs_settings = dict(_DEFAULT_SCHEDULER)
    if cloud_scheduler:
        cs_settings.update(cloud_scheduler)

    runtime_sa = service_account(
        name = "{}_runtime".format(name),
        project = project,
        account_id = runtime_account_id,
        display_name = "Runtime identity for cronjob {}".format(name),
    )

    scheduler_sa = service_account(
        name = "{}_scheduler".format(name),
        project = project,
        account_id = scheduler_account_id,
        display_name = "Cloud Scheduler invoker for {}".format(name),
    )

    invoker = project_iam_member(
        name = "{}_scheduler_invoker".format(name),
        project = project,
        role = "roles/run.invoker",
        member = "serviceAccount:{}".format(scheduler_sa.email),
    )

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

    container = {
        "image": image,
        "args": list(args),
        "resources": [{"limits": {
            "cpu": str(resources["cpu"]),
            "memory": "{}Mi".format(resources["memory"]),
        }}],
    }
    if env_blocks:
        container["env"] = env_blocks

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

    retry_config = None
    if cs_settings["retry_count"] > 0:
        retry_config = {"retry_count": cs_settings["retry_count"]}

    attempt_deadline = None
    if cs_settings["attempt_deadline_seconds"] != None:
        attempt_deadline = "{}s".format(cs_settings["attempt_deadline_seconds"])

    trigger = google_cloud_scheduler_job(
        name = name,
        project = project,
        region = region,
        scheduler_name = "{}-trigger".format(job_name),
        schedule = schedule["cron"],
        time_zone = schedule["time_zone"],
        http_target = {
            "http_method": "POST",
            "uri": "https://{region}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/{project}/jobs/{job_name}:run".format(
                region = region,
                project = project,
                job_name = job_name,
            ),
            "oauth_token": [{"service_account_email": scheduler_sa.email}],
        },
        attempt_deadline = attempt_deadline,
        retry_config = retry_config,
    )

    # Combine resource blocks. `service_account` appears twice (runtime +
    # scheduler) so a flat dict-merge on the rtype level would lose one;
    # merge at both rtype and instance-name levels.
    combined = {}
    for piece in (runtime_sa, scheduler_sa, invoker, job_resource, trigger):
        for rtype, instances in piece.tf["resource"].items():
            if rtype not in combined:
                combined[rtype] = {}
            for inst_name, body in instances.items():
                combined[rtype][inst_name] = body

    return struct(
        tf = {"resource": combined},
        addr = job_resource.addr,
        id = job_resource.id,
        name = job_resource.name,
        location = job_resource.location,
        service_account_email = runtime_sa.email,
        scheduler_service_account_email = scheduler_sa.email,
    )
