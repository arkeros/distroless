"""Starlark form of the bifrost `cronjob_kubernetes` module.

Cloud-Run-Job's batch sibling running on a Kubernetes cluster: a CronJob
applied via SSA, a runtime GSA bound by Workload Identity, and an
optional content-hashed K8s Secret for env material sourced from GCP
Secret Manager via ephemeral resources.

The bifrost macro returns one struct holding every resource so callers
drop it directly into `tf_root(docs=...)`.
"""

load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "ephemeral_google_secret_manager_secret_version",
    "service_account",
    "service_account_iam_member",
)
load(
    "//devtools/build/tools/tf/resources:k8s.bzl",
    "kubernetes_manifest",
    "kubernetes_secret_v1",
)

_DEFAULT_JOB = {
    "parallelism": 1,
    "completions": 1,
    "max_retries": 3,
    "timeout_seconds": 600,
}

_CONTAINER_NAME = "app"

_CONTAINER_SECURITY_CONTEXT = {
    "runAsNonRoot": True,
    "allowPrivilegeEscalation": False,
    "readOnlyRootFilesystem": True,
    "capabilities": {"drop": ["ALL"]},
    "seccompProfile": {"type": "RuntimeDefault"},
}

_POD_SECURITY_CONTEXT = {
    "runAsNonRoot": True,
    "seccompProfile": {"type": "RuntimeDefault"},
}

def _secret_env_signature(secret_env):
    """Stable 32-bit positive int hash of secret_env content.

    Drives both the K8s Secret name suffix and `data_wo_revision`, so a
    change to any (project, secret, version) forces the Secret to be
    recreated with new material — Kubernetes won't propagate a Secret
    whose data field is write-only on its own.

    Sort keys for determinism (Starlark dict iteration order tracks
    insertion order, which the caller does not control).
    """
    parts = []
    for k in sorted(secret_env.keys()):
        v = secret_env[k]
        parts.append(json.encode({
            "k": k,
            "project": v["project"],
            "secret": v["secret"],
            "version": v["version"],
        }))
    raw = hash("\n".join(parts))

    # Starlark hash() returns a signed 32-bit int. Force positive so the
    # value formats as a clean hex string and fits in float64 as the
    # data_wo_revision (terraform stores the revision as a number).
    if raw < 0:
        raw = raw + (1 << 32)
    return raw

def _merge_resource_blocks(structs):
    """Combine `tf` bodies from multiple resource structs.

    Resource sub-trees can collide at L1 (same rtype, different L2 names —
    e.g. two `google_service_account`s, or many `kubernetes_manifest`s in
    one module), so a flat dict-update at L1 would lose siblings. Merge
    at the L2 (instance-name) level instead.

    Ephemeral blocks live under L0 = "ephemeral" rather than "resource";
    handle both with the same shape.
    """
    merged = {}
    for s in structs:
        for l0_key, by_rtype in s.tf.items():
            section = merged.setdefault(l0_key, {})
            for rtype, instances in by_rtype.items():
                bucket = section.setdefault(rtype, {})
                for inst_name, body in instances.items():
                    bucket[inst_name] = body
    return merged

def cronjob_kubernetes(
        name,
        project,
        namespace,
        image,
        schedule,
        resources,
        job_name = None,
        args = (),
        env = None,
        secret_env = None,
        job = None,
        service_account_id = None,
        labels = None,
        field_manager = "terraform"):
    """K8s CronJob + runtime GSA + Workload Identity binding (+ optional Secret).

    Returns one struct whose `.tf` body holds:
      - `google_service_account.<name>_runtime`
      - `google_service_account_iam_member.<name>_workload_identity`
      - `kubernetes_manifest.<name>_service_account` (the K8s SA)
      - `kubernetes_manifest.<name>_cron_job` (the CronJob)
      - When `secret_env` is non-empty:
        - `ephemeral.google_secret_manager_secret_version.<name>_env_<k>` per entry
        - `kubernetes_secret_v1.<name>_env`

    Attribute refs on the returned struct: the runtime GSA email is
    exposed as `service_account_email`; the K8s SA name and CronJob
    name are exposed as `kubernetes_service_account_name` and
    `cron_job_name`.

    Args:
        name: Terraform block-key prefix; resources derive from this.
        project: GCP project for the runtime GSA, secrets, and the WI
            principalSet.
        namespace: Kubernetes namespace. The WI principalSet binds to
            `<project>.svc.id.goog[<namespace>/<job_name>]`.
        image: Container image, digest-pinned.
        schedule: `{cron: str, time_zone: str}`.
        resources: `{cpu: number, memory: int (MiB)}`. Both CPU and memory
            are set as request==limit. Web's "requests-only" rule does
            not apply to batch.
        job_name: K8s CronJob and SA name (kebab-case, K8s rules).
            Defaults to `name`. SA account_ids derive from `job_name`,
            so callers whose `name` is snake_case (TF identifier) should
            pass `job_name` in kebab-case explicitly.
        args: Container args list.
        env: Optional `{name: value}` for plain env vars.
        secret_env: Optional `{name: {project, secret, version}}` for
            Secret Manager-backed env. Version must be a numeric string.
        job: Optional `{parallelism, completions, max_retries, timeout_seconds}`
            overrides.
        service_account_id: Override the runtime GSA `account_id`.
            Defaults to `crj-<job_name>`.
        labels: Extra labels merged with `{app.kubernetes.io/name: <job_name>}`.
        field_manager: SSA field-manager name. Default `"terraform"`.
    """
    job_name = job_name or name
    runtime_account_id = service_account_id or "crj-{}".format(job_name)
    job_settings = dict(_DEFAULT_JOB)
    if job:
        job_settings.update(job)

    final_labels = {"app.kubernetes.io/name": job_name}
    if labels:
        final_labels.update(labels)

    # Runtime GSA + Workload Identity binding (project-side) ----------------
    runtime_sa = service_account(
        name = "{}_runtime".format(name),
        project = project,
        account_id = runtime_account_id,
        display_name = "Runtime identity for cronjob {}".format(job_name),
    )

    wi_binding = service_account_iam_member(
        name = "{}_workload_identity".format(name),
        service_account_id = runtime_sa.id,
        role = "roles/iam.workloadIdentityUser",
        member = "serviceAccount:{}.svc.id.goog[{}/{}]".format(
            project,
            namespace,
            job_name,
        ),
    )

    # K8s ServiceAccount (SSA) -----------------------------------------------
    k8s_sa = kubernetes_manifest(
        name = "{}_service_account".format(name),
        force_conflicts = True,
        field_manager_name = field_manager,
        manifest = {
            "apiVersion": "v1",
            "kind": "ServiceAccount",
            "metadata": {
                "name": job_name,
                "namespace": namespace,
                "labels": final_labels,
                "annotations": {"iam.gke.io/gcp-service-account": runtime_sa.email},
            },
        },
    )

    # Secret material (only when secret_env is non-empty) --------------------
    pieces = [runtime_sa, wi_binding, k8s_sa]
    secret_dependency = None

    if secret_env:
        signature = _secret_env_signature(secret_env)
        secret_name = "%s-env-%x" % (job_name, signature)

        # One ephemeral block per secret entry. Terraform's HCL form uses
        # `for_each`; with the content known at Starlark eval time we can
        # just emit them directly, which keeps the JSON readable and means
        # data_wo's references resolve to a concrete address.
        ephemeral_refs = {}
        for k in sorted(secret_env.keys()):
            v = secret_env[k]
            block_name = "{}_env_{}".format(name, k)
            eph = ephemeral_google_secret_manager_secret_version(
                name = block_name,
                project = v["project"],
                secret = v["secret"],
                version = v["version"],
            )
            pieces.append(eph)
            ephemeral_refs[k] = eph.secret_data

        env_secret = kubernetes_secret_v1(
            name = "{}_env".format(name),
            metadata = {
                "name": secret_name,
                "namespace": namespace,
                "labels": final_labels,
            },
            data_wo = ephemeral_refs,
            data_wo_revision = signature,
            create_before_destroy = True,
        )
        pieces.append(env_secret)
        secret_dependency = env_secret.addr

    # CronJob (SSA) ----------------------------------------------------------
    cpu_quantity = "{}m".format(int(resources["cpu"] * 1000))
    memory_quantity = "{}Mi".format(resources["memory"])

    container = {
        "name": _CONTAINER_NAME,
        "image": image,
        "args": list(args),
        "securityContext": _CONTAINER_SECURITY_CONTEXT,
        "resources": {
            "requests": {"cpu": cpu_quantity, "memory": memory_quantity},
            "limits": {"cpu": cpu_quantity, "memory": memory_quantity},
        },
    }
    if env:
        container["env"] = [{"name": k, "value": v} for k, v in env.items()]
    if secret_env:
        container["envFrom"] = [{"secretRef": {"name": secret_name}}]

    cron_depends_on = [k8s_sa.addr]
    if secret_dependency:
        cron_depends_on.append(secret_dependency)

    cron_job = kubernetes_manifest(
        name = "{}_cron_job".format(name),
        field_manager_name = field_manager,
        force_conflicts = False,
        # Image tag and labels/annotations get rewritten by the cluster's
        # push controller after the initial apply; computed_fields tells
        # terraform to ignore drift on those paths.
        computed_fields = [
            "metadata.annotations",
            "metadata.labels",
            "spec.jobTemplate.spec.template.spec.containers[0].image",
        ],
        depends_on = cron_depends_on,
        manifest = {
            "apiVersion": "batch/v1",
            "kind": "CronJob",
            "metadata": {
                "name": job_name,
                "namespace": namespace,
                "labels": final_labels,
            },
            "spec": {
                "schedule": schedule["cron"],
                "timeZone": schedule["time_zone"],
                "concurrencyPolicy": "Forbid",
                "successfulJobsHistoryLimit": 3,
                "failedJobsHistoryLimit": 1,
                "jobTemplate": {
                    "metadata": {"labels": final_labels},
                    "spec": {
                        "parallelism": job_settings["parallelism"],
                        "completions": job_settings["completions"],
                        "backoffLimit": job_settings["max_retries"],
                        "activeDeadlineSeconds": job_settings["timeout_seconds"],
                        "template": {
                            "metadata": {"labels": final_labels},
                            "spec": {
                                "serviceAccountName": job_name,
                                "restartPolicy": "Never",
                                "securityContext": _POD_SECURITY_CONTEXT,
                                "containers": [container],
                            },
                        },
                    },
                },
            },
        },
    )
    pieces.append(cron_job)

    combined = _merge_resource_blocks(pieces)

    return struct(
        tf = combined,
        addr = cron_job.addr,
        service_account_email = runtime_sa.email,
        kubernetes_service_account_name = job_name,
        cron_job_name = job_name,
    )
