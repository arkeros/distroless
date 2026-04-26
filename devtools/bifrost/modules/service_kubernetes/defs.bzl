"""Starlark form of the bifrost `service_kubernetes` module.

A Kubernetes web workload: Deployment + Service + HPA, with the same
SSA/typed split for secrets as cronjob_kubernetes (ephemeral Secret
Manager versions feed a content-hashed `kubernetes_secret_v1` via
`data_wo`). Optional PodDisruptionBudget when running >= 2 replicas;
optional VerticalPodAutoscaler scoped to memory.

Web resource policy (different from batch):
  - `requests.cpu` set, NO `limits.cpu`. CPU throttling is the wrong
    failure mode for request-serving workloads.
  - `requests.memory == limits.memory`.
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

_DEFAULT_AUTOSCALING_TARGET_CPU = 80

def _secret_env_signature(secret_env):
    """Stable 32-bit positive int hash of secret_env content.

    See cronjob_kubernetes/defs.bzl for the rationale; same approach.
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

def _merge_resource_blocks(structs):
    """Combine `tf` bodies from multiple resource structs.

    Two-deep merge: L0 (resource/ephemeral) → rtype → instance name.
    Same shape as cronjob_kubernetes/defs.bzl.
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
        field_manager = "terraform",
        depends_on = None):
    """Deployment + Service + HPA (+optional Secret/PDB/VPA) for a K8s web workload.

    Returns one struct whose `.tf` body holds:
      - `google_service_account.<name>_runtime`
      - `google_service_account_iam_member.<name>_workload_identity`
      - `kubernetes_manifest.<name>_service_account`
      - When `secret_env` is non-empty:
        - `ephemeral.google_secret_manager_secret_version.<name>_env_<k>` per entry
        - `kubernetes_secret_v1.<name>_env`
      - `kubernetes_manifest.<name>_deployment`
      - `kubernetes_manifest.<name>_service`
      - `kubernetes_manifest.<name>_hpa`
      - When `autoscaling.min >= 2`: `kubernetes_manifest.<name>_pdb`
      - When `vpa_enabled` (default True): `kubernetes_manifest.<name>_vpa`

    Attribute refs on the returned struct: the runtime GSA email is
    exposed as `service_account_email`; the K8s SA / Deployment /
    Service names are exposed as `kubernetes_service_account_name`,
    `deployment_name`, `service_name`.

    Args:
        name: Terraform block-key prefix; resources derive from this.
        project: GCP project for the runtime GSA, secrets, and the WI
            principalSet.
        namespace: Kubernetes namespace. The WI principalSet binds to
            `<project>.svc.id.goog[<namespace>/<workload_name>]`.
        image: Container image, digest-pinned.
        resources: `{cpu: number, memory: int (MiB)}`. `requests.cpu` set
            (no CPU limit); `requests.memory == limits.memory`.
        autoscaling: `{min: int, max: int, target_cpu_utilization: int}`.
            `target_cpu_utilization` defaults to 80. `min` must be >= 1.
            `min >= 2` adds a PodDisruptionBudget (`maxUnavailable = 1`).
        workload_name: K8s object name (Deployment / Service / SA), and
            the basis for the GSA `account_id`. Defaults to `name`.
            Pass kebab-case explicitly when `name` is snake_case.
        port: Container port; also added as the `PORT` env var.
        args: Container args list.
        env: Optional `{name: value}` for plain env vars. Merged on top
            of `{"PORT": str(port)}`.
        secret_env: Optional `{name: {project, secret, version}}` for
            Secret Manager-backed env. Version must be a numeric string.
        probes: Optional `{startup_path, liveness_path, readiness_path}`.
            Each missing key skips that probe.
        vpa_enabled: Emit a VerticalPodAutoscaler for memory on the
            `app` container. Default True. Requires the VPA CRD; set
            False on clusters without it.
        service_account_id: Override the runtime GSA `account_id`.
            Defaults to `svc-<workload_name>`.
        labels: Extra labels merged with `{app.kubernetes.io/name: <workload_name>}`.
        field_manager: SSA field-manager name. Default `"terraform"`.
        depends_on: Optional list of terraform addresses (e.g.
            `[migrate.addr]`) appended to the Deployment's `depends_on`.
            Use this for sibling resources that must complete before pods
            roll out — typically a one-shot migration Job.
    """
    workload_name = workload_name or name
    runtime_account_id = service_account_id or "svc-{}".format(workload_name)

    selector_labels = {"app.kubernetes.io/name": workload_name}
    final_labels = dict(selector_labels)
    if labels:
        final_labels.update(labels)

    target_cpu = autoscaling.get("target_cpu_utilization", _DEFAULT_AUTOSCALING_TARGET_CPU)
    pdb_enabled = autoscaling["min"] >= 2

    # ── GCP identity ────────────────────────────────────────────────────
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
        member = "serviceAccount:{}.svc.id.goog[{}/{}]".format(
            project,
            namespace,
            workload_name,
        ),
    )

    # ── K8s ServiceAccount (SSA) ────────────────────────────────────────
    k8s_sa = kubernetes_manifest(
        name = "{}_service_account".format(name),
        force_conflicts = True,
        field_manager_name = field_manager,
        manifest = {
            "apiVersion": "v1",
            "kind": "ServiceAccount",
            "metadata": {
                "name": workload_name,
                "namespace": namespace,
                "labels": final_labels,
                "annotations": {"iam.gke.io/gcp-service-account": runtime_sa.email},
            },
        },
    )

    # ── Secret material (only when secret_env is non-empty) ─────────────
    pieces = [runtime_sa, wi_binding, k8s_sa]
    secret_dependency = None
    secret_name = None

    if secret_env:
        signature = _secret_env_signature(secret_env)
        secret_name = "%s-env-%x" % (workload_name, signature)

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

    # ── Container spec ──────────────────────────────────────────────────
    cpu_request = "{}m".format(int(resources["cpu"] * 1000))
    memory_quantity = "{}Mi".format(resources["memory"])

    full_env = {"PORT": str(port)}
    if env:
        full_env.update(env)
    container_env = [{"name": k, "value": v} for k, v in full_env.items()]

    container = {
        "name": _CONTAINER_NAME,
        "image": image,
        "args": list(args),
        "ports": [{"containerPort": port}],
        "securityContext": _CONTAINER_SECURITY_CONTEXT,
        "env": container_env,
        # Web policy: requests.cpu only (no CPU limit), memory request==limit.
        "resources": {
            "requests": {"cpu": cpu_request, "memory": memory_quantity},
            "limits": {"memory": memory_quantity},
        },
    }
    if secret_env:
        container["envFrom"] = [{"secretRef": {"name": secret_name}}]
    if probes:
        if probes.get("startup_path"):
            container["startupProbe"] = {
                "httpGet": {"path": probes["startup_path"], "port": port},
            }
        if probes.get("liveness_path"):
            container["livenessProbe"] = {
                "httpGet": {"path": probes["liveness_path"], "port": port},
            }
        if probes.get("readiness_path"):
            container["readinessProbe"] = {
                "httpGet": {"path": probes["readiness_path"], "port": port},
            }

    # ── Deployment ──────────────────────────────────────────────────────
    deployment_depends_on = [k8s_sa.addr]
    if secret_dependency:
        deployment_depends_on.append(secret_dependency)
    if depends_on:
        deployment_depends_on = deployment_depends_on + list(depends_on)

    deployment = kubernetes_manifest(
        name = "{}_deployment".format(name),
        field_manager_name = field_manager,
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
            "metadata": {
                "name": workload_name,
                "namespace": namespace,
                "labels": final_labels,
            },
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

    # ── Service (ClusterIP) ─────────────────────────────────────────────
    service = kubernetes_manifest(
        name = "{}_service".format(name),
        force_conflicts = True,
        field_manager_name = field_manager,
        manifest = {
            "apiVersion": "v1",
            "kind": "Service",
            "metadata": {
                "name": workload_name,
                "namespace": namespace,
                "labels": final_labels,
            },
            "spec": {
                "type": "ClusterIP",
                "selector": selector_labels,
                "ports": [{
                    "name": "http",
                    "port": port,
                    "targetPort": port,
                }],
            },
        },
    )
    pieces.append(service)

    # ── HPA ─────────────────────────────────────────────────────────────
    hpa = kubernetes_manifest(
        name = "{}_hpa".format(name),
        force_conflicts = True,
        field_manager_name = field_manager,
        depends_on = [deployment.addr],
        manifest = {
            "apiVersion": "autoscaling/v2",
            "kind": "HorizontalPodAutoscaler",
            "metadata": {
                "name": workload_name,
                "namespace": namespace,
                "labels": final_labels,
            },
            "spec": {
                "minReplicas": autoscaling["min"],
                "maxReplicas": autoscaling["max"],
                "scaleTargetRef": {
                    "apiVersion": "apps/v1",
                    "kind": "Deployment",
                    "name": workload_name,
                },
                "metrics": [{
                    "type": "Resource",
                    "resource": {
                        "name": "cpu",
                        "target": {
                            "type": "Utilization",
                            "averageUtilization": target_cpu,
                        },
                    },
                }],
            },
        },
    )
    pieces.append(hpa)

    # ── PDB (only when min >= 2) ────────────────────────────────────────
    if pdb_enabled:
        pdb = kubernetes_manifest(
            name = "{}_pdb".format(name),
            force_conflicts = True,
            field_manager_name = field_manager,
            manifest = {
                "apiVersion": "policy/v1",
                "kind": "PodDisruptionBudget",
                "metadata": {
                    "name": workload_name,
                    "namespace": namespace,
                    "labels": final_labels,
                },
                "spec": {
                    "maxUnavailable": 1,
                    "selector": {"matchLabels": selector_labels},
                },
            },
        )
        pieces.append(pdb)

    # ── VPA (memory-only) ───────────────────────────────────────────────
    if vpa_enabled:
        vpa = kubernetes_manifest(
            name = "{}_vpa".format(name),
            force_conflicts = True,
            field_manager_name = field_manager,
            depends_on = [deployment.addr],
            manifest = {
                "apiVersion": "autoscaling.k8s.io/v1",
                "kind": "VerticalPodAutoscaler",
                "metadata": {
                    "name": workload_name,
                    "namespace": namespace,
                    "labels": final_labels,
                },
                "spec": {
                    "targetRef": {
                        "apiVersion": "apps/v1",
                        "kind": "Deployment",
                        "name": workload_name,
                    },
                    "updatePolicy": {"updateMode": "InPlaceOrRecreate"},
                    "resourcePolicy": {
                        "containerPolicies": [{
                            "containerName": _CONTAINER_NAME,
                            "controlledResources": ["memory"],
                        }],
                    },
                },
            },
        )
        pieces.append(vpa)

    combined = _merge_resource_blocks(pieces)

    return struct(
        tf = combined,
        addr = deployment.addr,
        service_account_email = runtime_sa.email,
        kubernetes_service_account_name = workload_name,
        deployment_name = workload_name,
        service_name = workload_name,
    )
