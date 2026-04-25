"""GCP resource constructors.

Each function returns a struct compatible with `tf_root(docs=...)`:
- `.tf` is the JSON body for the resource/provider/data block
- `.addr` is the bare Terraform address (for `depends_on`)
- One named field per readable attribute (interpolation strings)

Add new constructors as roots need them. Keep the attrs lists tight — every
field added here is a piece of the resource's schema we're claiming exists.
"""

load("//devtools/build/tools/tf:defs.bzl", "resource")

# ---------- providers -------------------------------------------------------

def google_provider(project, region = None, **kwargs):
    """The `google` provider block. Bare `provider "google" { ... }`.

    Aliased instances (multiple regions, multiple accounts) are not supported
    yet — add an `alias` parameter when needed.
    """
    body = {"project": project}
    if region != None:
        body["region"] = region
    body.update(kwargs)
    return struct(tf = {"provider": {"google": body}})

# ---------- project-level ---------------------------------------------------

def project_service(name, project, service, disable_on_destroy = True):
    """`google_project_service` — enable a GCP API on a project.

    `disable_on_destroy = False` is the right default for shared APIs, since
    a destroy here shouldn't disable APIs other roots depend on. We default
    to True (terraform's default) and let the caller override for shared
    services.
    """
    return resource(
        rtype = "google_project_service",
        name = name,
        body = {
            "project": project,
            "service": service,
            "disable_on_destroy": disable_on_destroy,
        },
        attrs = ["id"],
    )

# ---------- artifact registry -----------------------------------------------

def service_account(name, project, account_id, display_name = None):
    """`google_service_account`."""
    body = {
        "project": project,
        "account_id": account_id,
    }
    if display_name != None:
        body["display_name"] = display_name
    return resource(
        rtype = "google_service_account",
        name = name,
        body = body,
        attrs = ["email", "id", "name", "unique_id", "member", "account_id"],
    )

# ---------------------------------------------------------------------------
# Cloud Run v2: 1:1 resource wrappers.
#
# The convenience composer that builds a full Cloud Run service from
# bifrost-style flat inputs lives next to its HCL twin at
# `//devtools/bifrost/terraform/modules/service_cloudrun:defs.bzl`.
# ---------------------------------------------------------------------------

_CLOUD_RUN_V2_SERVICE_ATTRS = ("uri", "id", "name", "location")
_IAM_MEMBER_ATTRS = ("id", "etag")

def google_cloud_run_v2_service(
        name,
        location,
        project = None,
        service_name = None,
        ingress = None,
        labels = None,
        annotations = None,
        description = None,
        custom_audiences = None,
        deletion_protection = None,
        invoker_iam_disabled = None,
        launch_stage = None,
        template = None,
        traffic = None,
        scaling = None,
        binary_authorization = None,
        depends_on = None):
    """`google_cloud_run_v2_service` — Cloud Run v2 service.

    `name` is the Terraform block key; `service_name` is the TF schema's
    `name` field (the Cloud Run service name) and defaults to the block key.
    Nested blocks (`template`, `traffic`, `scaling`, `binary_authorization`)
    are passed as dicts/lists shaped like Terraform JSON; for the convenience
    macro that builds them from flat kwargs, see `cloud_run_service`.
    """
    body = {
        "location": location,
        "name": service_name or name,
    }
    if project != None:
        body["project"] = project
    if ingress != None:
        body["ingress"] = ingress
    if labels != None:
        body["labels"] = labels
    if annotations != None:
        body["annotations"] = annotations
    if description != None:
        body["description"] = description
    if custom_audiences != None:
        body["custom_audiences"] = custom_audiences
    if deletion_protection != None:
        body["deletion_protection"] = deletion_protection
    if invoker_iam_disabled != None:
        body["invoker_iam_disabled"] = invoker_iam_disabled
    if launch_stage != None:
        body["launch_stage"] = launch_stage
    if template != None:
        body["template"] = template if type(template) == type([]) else [template]
    if traffic != None:
        body["traffic"] = traffic
    if scaling != None:
        body["scaling"] = scaling
    if binary_authorization != None:
        body["binary_authorization"] = binary_authorization
    if depends_on != None:
        body["depends_on"] = depends_on
    return resource(
        rtype = "google_cloud_run_v2_service",
        name = name,
        body = body,
        attrs = _CLOUD_RUN_V2_SERVICE_ATTRS,
    )

def google_cloud_run_v2_service_iam_member(
        name,
        location,
        service_name,
        role,
        member,
        project = None,
        condition = None,
        depends_on = None):
    """`google_cloud_run_v2_service_iam_member` — single IAM principal binding.

    `name` is the Terraform block key; `service_name` is the TF schema's
    `name` field (the target Cloud Run service's name).
    """
    body = {
        "location": location,
        "name": service_name,
        "role": role,
        "member": member,
    }
    if project != None:
        body["project"] = project
    if condition != None:
        body["condition"] = condition
    if depends_on != None:
        body["depends_on"] = depends_on
    return resource(
        rtype = "google_cloud_run_v2_service_iam_member",
        name = name,
        body = body,
        attrs = _IAM_MEMBER_ATTRS,
    )

# ---------- artifact registry -----------------------------------------------

def artifact_registry_repository(
        name,
        project,
        location,
        repository_id,
        format,
        description = None,
        depends_on = None):
    """`google_artifact_registry_repository`."""
    body = {
        "project": project,
        "location": location,
        "repository_id": repository_id,
        "format": format,
    }
    if description != None:
        body["description"] = description
    if depends_on != None:
        body["depends_on"] = depends_on
    return resource(
        rtype = "google_artifact_registry_repository",
        name = name,
        body = body,
        attrs = ["id", "name", "location", "repository_id", "format"],
    )
