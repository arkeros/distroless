"""Kubernetes resource constructors.

The kubernetes provider has two camps and we use both:

- `kubernetes_manifest` — server-side-apply against the API server with the
  YAML-equivalent dict as `manifest`. Use for everything except secrets
  (the dynamic `manifest` attribute can't carry write-only semantics).
- `kubernetes_secret_v1` — typed resource with `data_wo` / `data_wo_revision`
  for writing ephemeral secret material without persisting plaintext in
  terraform state.

Same struct contract as `gcp.bzl`: `.tf` is the JSON body, `.addr` the bare
Terraform address. Add new constructors as bifrost modules need them.
"""

load("//devtools/build/tools/tf:defs.bzl", "resource")

def kubernetes_manifest(
        name,
        manifest,
        field_manager_name = "terraform",
        force_conflicts = False,
        computed_fields = None,
        depends_on = None):
    """`kubernetes_manifest` — server-side-apply a dict-shaped K8s object.

    `manifest` is the YAML-equivalent dict (apiVersion, kind, metadata, spec).
    `force_conflicts = True` makes this manager win SSA conflicts; use it
    for objects we own end-to-end (ServiceAccounts), and leave it False for
    objects whose fields legitimately get mutated by other controllers
    (e.g. an image tag rewritten by a push controller — list those in
    `computed_fields` so terraform doesn't fight them).
    """
    body = {
        "field_manager": [{
            "name": field_manager_name,
            "force_conflicts": force_conflicts,
        }],
        "manifest": manifest,
    }
    if computed_fields != None:
        body["computed_fields"] = computed_fields
    if depends_on != None:
        body["depends_on"] = depends_on
    return resource(
        rtype = "kubernetes_manifest",
        name = name,
        body = body,
        attrs = ("manifest",),
    )

def kubernetes_job_v1(
        name,
        metadata,
        spec,
        wait_for_completion = None,
        depends_on = None):
    """`kubernetes_job_v1` — typed (non-SSA) K8s Job.

    Useful for migration jobs that must complete before a Deployment rolls
    out: pass `wait_for_completion = True` and list this resource in the
    consumer's `depends_on`. Typed-resource semantics: terraform tracks
    completion state via the K8s API rather than via SSA.

    `metadata` and `spec` accept either a single dict (wrapped) or an
    already-shaped list-of-one for callers passing the JSON shape directly.
    """
    body = {
        "metadata": metadata if type(metadata) == type([]) else [metadata],
        "spec": spec if type(spec) == type([]) else [spec],
    }
    if wait_for_completion != None:
        body["wait_for_completion"] = wait_for_completion
    if depends_on != None:
        body["depends_on"] = depends_on
    return resource(
        rtype = "kubernetes_job_v1",
        name = name,
        body = body,
        attrs = ("id",),
    )

def kubernetes_secret_v1(
        name,
        metadata,
        data_wo = None,
        data_wo_revision = None,
        secret_type = None,
        depends_on = None,
        create_before_destroy = None):
    """`kubernetes_secret_v1` — typed K8s Secret with write-only data.

    Pass `data_wo` (write-only data, never read back into state) and
    `data_wo_revision` (an integer that must change to trigger a rewrite).
    A monotonically-increasing revision is the wrong semantic here — what
    you want is "rewrite when the upstream secret material changes",
    which is best computed as a stable hash of the inputs.

    `create_before_destroy = True` is the right default when the secret
    name itself encodes the content hash: rolling forward to a new name
    must materialize the new Secret before the old one is removed,
    otherwise pods reference a Secret that doesn't exist.

    `metadata` accepts either a single dict (wrapped) or an already-shaped
    list-of-one for callers that want to pass the JSON shape directly.
    """
    body = {
        "metadata": metadata if type(metadata) == type([]) else [metadata],
    }
    if secret_type != None:
        body["type"] = secret_type
    if data_wo != None:
        body["data_wo"] = data_wo
    if data_wo_revision != None:
        body["data_wo_revision"] = data_wo_revision
    if depends_on != None:
        body["depends_on"] = depends_on
    if create_before_destroy != None:
        body["lifecycle"] = [{"create_before_destroy": create_before_destroy}]
    return resource(
        rtype = "kubernetes_secret_v1",
        name = name,
        body = body,
        attrs = ("id", "metadata"),
    )
