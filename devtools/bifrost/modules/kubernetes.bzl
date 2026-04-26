"""Bifrost macros for Kubernetes web services and cronjobs.

`service_kubernetes(...)` — Deployment + Service + HPA, with optional
PodDisruptionBudget (when `autoscaling.min >= 2`) and memory-only VPA.
Web resource policy: `requests.cpu` set, NO `limits.cpu` (avoid CFS
throttling); `requests.memory == limits.memory`.

`cronjob_kubernetes(...)` — CronJob applied via SSA. Batch resource
policy: both `requests.cpu == limits.cpu` and `requests.memory ==
limits.memory`. The "no CPU limit" web rule does not apply to batch.

Both macros share: a runtime GSA + Workload Identity binding, a
content-hashed `kubernetes_secret_v1` for ephemeral Secret Manager
material, and a single set of hardened pod/container security contexts.
"""

load("//devtools/build/tools/tf:defs.bzl", "merge_tf")
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

# ── Constants ──────────────────────────────────────────────────────────────

_CONTAINER_NAME = "app"  # stable name so VPA resourcePolicy can target it.
_SELECTOR_LABEL = "app.kubernetes.io/name"
_GSA_ANNOTATION = "iam.gke.io/gcp-service-account"

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

_DEFAULT_JOB = {
    "parallelism": 1,
    "completions": 1,
    "max_retries": 3,
    "timeout_seconds": 600,
}

_DEFAULT_AUTOSCALING_TARGET_CPU = 80

# ── Shared helpers ─────────────────────────────────────────────────────────

def _labels(workload_name, extra):
    """`{app.kubernetes.io/name: <workload_name>}` plus any extras."""
    out = {_SELECTOR_LABEL: workload_name}
    if extra:
        out.update(extra)
    return out

def _selector(workload_name):
    return {_SELECTOR_LABEL: workload_name}

def _secret_env_signature(secret_env):
    """Stable 32-bit positive int hash of secret_env content.

    Drives both the `kubernetes_secret_v1` name suffix and `data_wo_revision`,
    so a change to any (project, secret, version) forces the Secret to be
    recreated with new material — Kubernetes won't propagate a Secret whose
    data field is write-only on its own.

    Sort keys for determinism (Starlark dict iteration tracks insertion
    order, which the caller does not control). `hash()` is documented as
    Java-hashCode-based and stable across Bazel versions.
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
    if raw < 0:
        raw = raw + (1 << 32)
    return raw

def _runtime_identity(name, project, namespace, workload_name, account_id_prefix, account_id_override, labels):
    """Runtime GSA + Workload Identity binding + the K8s ServiceAccount.

    Returns three structs (runtime_sa, wi_binding, k8s_sa) ready to drop
    into a `merge_tf(...)` bundle. The K8s SA is annotated with the GSA
    email so Workload Identity flows through.
    """
    runtime_account_id = account_id_override or "{}-{}".format(account_id_prefix, workload_name)

    runtime_sa = service_account(
        name = "{}_runtime".format(name),
        project = project,
        account_id = runtime_account_id,
        display_name = "Runtime identity for {}".format(workload_name),
    )

    wi_binding = service_account_iam_member(
        name = "{}_workload_identity".format(name),
        service_account_id = runtime_sa.id,
        role = "roles/iam.workloadIdentityUser",
        member = "serviceAccount:{}.svc.id.goog[{}/{}]".format(project, namespace, workload_name),
    )

    k8s_sa = kubernetes_manifest(
        name = "{}_service_account".format(name),
        force_conflicts = True,
        manifest = {
            "apiVersion": "v1",
            "kind": "ServiceAccount",
            "metadata": {
                "name": workload_name,
                "namespace": namespace,
                "labels": labels,
                "annotations": {_GSA_ANNOTATION: runtime_sa.email},
            },
        },
    )

    return runtime_sa, wi_binding, k8s_sa

def _secret_env_carveout(name, workload_name, namespace, secret_env, labels):
    """Materialize secret_env as ephemeral SM versions + a content-hashed K8s Secret.

    Returns `(pieces, secret_name)`. `pieces` is the list of resource
    structs to merge (one ephemeral block per entry, plus the Secret).
    `secret_name` is the metadata.name to reference from the Deployment's
    `envFrom.secretRef`.
    """
    signature = _secret_env_signature(secret_env)
    secret_name = "%s-env-%x" % (workload_name, signature)

    pieces = []
    ephemeral_refs = {}
    for k in sorted(secret_env.keys()):
        v = secret_env[k]
        eph = ephemeral_google_secret_manager_secret_version(
            name = "{}_env_{}".format(name, k),
            project = v["project"],
            secret = v["secret"],
            version = v["version"],
        )
        pieces.append(eph)
        ephemeral_refs[k] = eph.secret_data

    pieces.append(kubernetes_secret_v1(
        name = "{}_env".format(name),
        metadata = {
            "name": secret_name,
            "namespace": namespace,
            "labels": labels,
        },
        data_wo = ephemeral_refs,
        data_wo_revision = signature,
        create_before_destroy = True,
    ))

    return pieces, secret_name

def _http_probe(path, port):
    return {"httpGet": {"path": path, "port": port}}

def _attach_probes(container, probes, port):
    """Attach startup/liveness/readiness probes to the container dict.

    Probes are optional individually — missing keys skip that probe.
    """
    if not probes:
        return
    if probes.get("startup_path"):
        container["startupProbe"] = _http_probe(probes["startup_path"], port)
    if probes.get("liveness_path"):
        container["livenessProbe"] = _http_probe(probes["liveness_path"], port)
    if probes.get("readiness_path"):
        container["readinessProbe"] = _http_probe(probes["readiness_path"], port)

# ── service_kubernetes ─────────────────────────────────────────────────────

def service_kubernetes(
        name,
        project,
        namespace,
        image,
        resources,
        autoscaling,
        workload_name = None,
        port = 8080,
        args = (),
        env = None,
        secret_env = None,
        probes = None,
        vpa_enabled = True,
        service_account_id = None,
        labels = None,
        depends_on = None):
    """Deployment + Service + HPA (+optional Secret/PDB/VPA) for a K8s web workload.

    Returns one struct exposing `service_account_email` (runtime GSA) plus
    `addr` for the Deployment.

    Args:
        name: Terraform block-key prefix; resources derive from this.
        project: GCP project for the runtime GSA, secrets, and the WI principalSet.
        namespace: Kubernetes namespace.
        image: Container image, digest-pinned.
        resources: `{cpu: number, memory: int (MiB)}`. `requests.cpu` set
            (no CPU limit); `requests.memory == limits.memory`.
        autoscaling: `{min: int, max: int, target_cpu_utilization: int}`.
            `target_cpu_utilization` defaults to 80; `min` must be >= 1.
            `min >= 2` adds a PodDisruptionBudget (`maxUnavailable = 1`).
        workload_name: K8s object name (Deployment / Service / SA), and
            the basis for the GSA `account_id`. Defaults to `name`.
            Pass kebab-case explicitly when `name` is snake_case.
        port: Container port; also added as the `PORT` env var.
        args: Container args list.
        env: Optional `{name: value}` for plain env vars; merged on top of
            `{"PORT": str(port)}`.
        secret_env: Optional `{name: {project, secret, version}}` for Secret
            Manager-backed env.
        probes: Optional `{startup_path, liveness_path, readiness_path}`.
        vpa_enabled: Emit a memory-only VPA. Default True. Requires the
            VPA CRD on the cluster.
        service_account_id: Override the runtime GSA `account_id`.
            Defaults to `svc-<workload_name>`.
        labels: Extra labels merged with `{app.kubernetes.io/name: <workload_name>}`.
        depends_on: Optional list of terraform addresses appended to the
            Deployment's `depends_on`. Use for sibling resources that must
            complete before pods roll out (e.g. a migration Job).
    """
    workload_name = workload_name or name
    final_labels = _labels(workload_name, labels)
    selector_labels = _selector(workload_name)
    target_cpu = autoscaling.get("target_cpu_utilization", _DEFAULT_AUTOSCALING_TARGET_CPU)
    pdb_enabled = autoscaling["min"] >= 2

    runtime_sa, wi_binding, k8s_sa = _runtime_identity(
        name, project, namespace, workload_name,
        account_id_prefix = "svc",
        account_id_override = service_account_id,
        labels = final_labels,
    )
    pieces = [runtime_sa, wi_binding, k8s_sa]
    deployment_depends_on = [k8s_sa.addr]

    secret_name = None
    if secret_env:
        secret_pieces, secret_name = _secret_env_carveout(name, workload_name, namespace, secret_env, final_labels)
        pieces.extend(secret_pieces)
        deployment_depends_on.append(secret_pieces[-1].addr)

    if depends_on:
        deployment_depends_on = deployment_depends_on + list(depends_on)

    cpu_request = "{}m".format(int(resources["cpu"] * 1000))
    memory_quantity = "{}Mi".format(resources["memory"])

    full_env = {"PORT": str(port)}
    if env:
        full_env.update(env)

    container = {
        "name": _CONTAINER_NAME,
        "image": image,
        "args": list(args),
        "ports": [{"containerPort": port}],
        "securityContext": _CONTAINER_SECURITY_CONTEXT,
        "env": [{"name": k, "value": v} for k, v in full_env.items()],
        # Web policy: requests.cpu only (no CPU limit), memory request==limit.
        "resources": {
            "requests": {"cpu": cpu_request, "memory": memory_quantity},
            "limits": {"memory": memory_quantity},
        },
    }
    if secret_name:
        container["envFrom"] = [{"secretRef": {"name": secret_name}}]
    _attach_probes(container, probes, port)

    deployment = kubernetes_manifest(
        name = "{}_deployment".format(name),
        force_conflicts = False,
        # spec.replicas: HPA owns long-term, Flagger briefly during canary.
        # image stays out of computed_fields — Flagger doesn't write back to
        # this Deployment (it mutates its own `-primary` sibling).
        computed_fields = [
            "metadata.annotations",
            "metadata.labels",
            "spec.replicas",
        ],
        depends_on = deployment_depends_on,
        manifest = {
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {"name": workload_name, "namespace": namespace, "labels": final_labels},
            "spec": {
                "selector": {"matchLabels": selector_labels},
                "template": {
                    "metadata": {"labels": final_labels},
                    "spec": {
                        "serviceAccountName": workload_name,
                        "securityContext": _POD_SECURITY_CONTEXT,
                        "containers": [container],
                    },
                },
            },
        },
    )
    pieces.append(deployment)

    pieces.append(kubernetes_manifest(
        name = "{}_service".format(name),
        force_conflicts = True,
        manifest = {
            "apiVersion": "v1",
            "kind": "Service",
            "metadata": {"name": workload_name, "namespace": namespace, "labels": final_labels},
            "spec": {
                "type": "ClusterIP",
                "selector": selector_labels,
                "ports": [{"name": "http", "port": port, "targetPort": port}],
            },
        },
    ))

    pieces.append(kubernetes_manifest(
        name = "{}_hpa".format(name),
        force_conflicts = True,
        depends_on = [deployment.addr],
        manifest = {
            "apiVersion": "autoscaling/v2",
            "kind": "HorizontalPodAutoscaler",
            "metadata": {"name": workload_name, "namespace": namespace, "labels": final_labels},
            "spec": {
                "minReplicas": autoscaling["min"],
                "maxReplicas": autoscaling["max"],
                "scaleTargetRef": {"apiVersion": "apps/v1", "kind": "Deployment", "name": workload_name},
                "metrics": [{
                    "type": "Resource",
                    "resource": {
                        "name": "cpu",
                        "target": {"type": "Utilization", "averageUtilization": target_cpu},
                    },
                }],
            },
        },
    ))

    if pdb_enabled:
        pieces.append(kubernetes_manifest(
            name = "{}_pdb".format(name),
            force_conflicts = True,
            manifest = {
                "apiVersion": "policy/v1",
                "kind": "PodDisruptionBudget",
                "metadata": {"name": workload_name, "namespace": namespace, "labels": final_labels},
                "spec": {"maxUnavailable": 1, "selector": {"matchLabels": selector_labels}},
            },
        ))

    if vpa_enabled:
        pieces.append(kubernetes_manifest(
            name = "{}_vpa".format(name),
            force_conflicts = True,
            depends_on = [deployment.addr],
            manifest = {
                "apiVersion": "autoscaling.k8s.io/v1",
                "kind": "VerticalPodAutoscaler",
                "metadata": {"name": workload_name, "namespace": namespace, "labels": final_labels},
                "spec": {
                    "targetRef": {"apiVersion": "apps/v1", "kind": "Deployment", "name": workload_name},
                    "updatePolicy": {"updateMode": "InPlaceOrRecreate"},
                    "resourcePolicy": {
                        "containerPolicies": [{
                            "containerName": _CONTAINER_NAME,
                            "controlledResources": ["memory"],
                        }],
                    },
                },
            },
        ))

    return struct(
        tf = merge_tf(*pieces),
        addr = deployment.addr,
        service_account_email = runtime_sa.email,
    )

# ── cronjob_kubernetes ─────────────────────────────────────────────────────

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
        labels = None):
    """K8s CronJob + runtime GSA + Workload Identity binding (+ optional Secret).

    Returns one struct exposing `service_account_email` (runtime GSA) plus
    `addr` for the CronJob.

    Args:
        name: Terraform block-key prefix; resources derive from this.
        project: GCP project for the runtime GSA, secrets, WI principalSet.
        namespace: Kubernetes namespace.
        image: Container image, digest-pinned.
        schedule: `{cron: str, time_zone: str}`.
        resources: `{cpu: number, memory: int (MiB)}`. Both CPU and memory
            are set as request==limit (the batch policy).
        job_name: K8s CronJob and SA name (kebab-case). Defaults to `name`.
            SA account_ids derive from `job_name`, so callers whose `name`
            is snake_case should pass `job_name` in kebab-case.
        args: Container args list.
        env: Optional `{name: value}` for plain env vars.
        secret_env: Optional `{name: {project, secret, version}}` for Secret
            Manager-backed env.
        job: Optional `{parallelism, completions, max_retries, timeout_seconds}`.
        service_account_id: Override the runtime GSA `account_id`.
            Defaults to `crj-<job_name>`.
        labels: Extra labels merged with `{app.kubernetes.io/name: <job_name>}`.
    """
    job_name = job_name or name
    final_labels = _labels(job_name, labels)

    job_settings = dict(_DEFAULT_JOB)
    if job:
        job_settings.update(job)

    runtime_sa, wi_binding, k8s_sa = _runtime_identity(
        name, project, namespace, job_name,
        account_id_prefix = "crj",
        account_id_override = service_account_id,
        labels = final_labels,
    )
    pieces = [runtime_sa, wi_binding, k8s_sa]
    cron_depends_on = [k8s_sa.addr]

    secret_name = None
    if secret_env:
        secret_pieces, secret_name = _secret_env_carveout(name, job_name, namespace, secret_env, final_labels)
        pieces.extend(secret_pieces)
        cron_depends_on.append(secret_pieces[-1].addr)

    cpu_quantity = "{}m".format(int(resources["cpu"] * 1000))
    memory_quantity = "{}Mi".format(resources["memory"])

    container = {
        "name": _CONTAINER_NAME,
        "image": image,
        "args": list(args),
        "securityContext": _CONTAINER_SECURITY_CONTEXT,
        # Batch policy: request == limit for both CPU and memory.
        "resources": {
            "requests": {"cpu": cpu_quantity, "memory": memory_quantity},
            "limits": {"cpu": cpu_quantity, "memory": memory_quantity},
        },
    }
    if env:
        container["env"] = [{"name": k, "value": v} for k, v in env.items()]
    if secret_name:
        container["envFrom"] = [{"secretRef": {"name": secret_name}}]

    cron_job = kubernetes_manifest(
        name = "{}_cron_job".format(name),
        force_conflicts = False,
        # Image tag and labels/annotations get rewritten by the cluster's
        # push controller after the initial apply.
        computed_fields = [
            "metadata.annotations",
            "metadata.labels",
            "spec.jobTemplate.spec.template.spec.containers[0].image",
        ],
        depends_on = cron_depends_on,
        manifest = {
            "apiVersion": "batch/v1",
            "kind": "CronJob",
            "metadata": {"name": job_name, "namespace": namespace, "labels": final_labels},
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

    return struct(
        tf = merge_tf(*pieces),
        addr = cron_job.addr,
        service_account_email = runtime_sa.email,
    )
