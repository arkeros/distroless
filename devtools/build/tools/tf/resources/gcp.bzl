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

def project_iam_audit_config(name, project, service, log_types):
    """`google_project_iam_audit_config` — Cloud Audit Logs Data Access config.

    Admin Activity logs are on by default and free; this resource is for
    enabling the streams that aren't (DATA_READ, DATA_WRITE, ADMIN_READ).
    `service = "allServices"` applies to every service, but is a footgun
    for cost — prefer naming services explicitly.

    `log_types` is a list of {"DATA_READ", "DATA_WRITE", "ADMIN_READ"}.
    """
    return resource(
        rtype = "google_project_iam_audit_config",
        name = name,
        body = {
            "project": project,
            "service": service,
            "audit_log_config": [
                {"log_type": lt}
                for lt in log_types
            ],
        },
        attrs = ["etag"],
    )

def logging_project_exclusion(name, project, exclusion_name, filter, description = None, disabled = False):
    """`google_logging_project_exclusion` — drop matching entries before
    they hit the `_Default` log bucket.

    Use to suppress high-volume / low-signal entries that would otherwise
    overwhelm Data Access logs (e.g. reads on a public 404 bucket).
    Excluded entries are *not* counted toward the project's log
    ingestion bill.
    """
    body = {
        "project": project,
        "name": exclusion_name,
        "filter": filter,
        "disabled": disabled,
    }
    if description != None:
        body["description"] = description
    return resource(
        rtype = "google_logging_project_exclusion",
        name = name,
        body = body,
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
# bifrost-style flat inputs lives at
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

# ---------- storage --------------------------------------------------------

def storage_bucket(
        name,
        project,
        bucket_name,
        location,
        uniform_bucket_level_access = None,
        force_destroy = None,
        lifecycle_rule = None,
        labels = None,
        public_access_prevention = None,
        versioning = None):
    """`google_storage_bucket`.

    `bucket_name` is the GCS bucket name (the TF schema's `name` field).
    `name` is the Terraform resource block key.
    """
    body = {
        "project": project,
        "name": bucket_name,
        "location": location,
    }
    if uniform_bucket_level_access != None:
        body["uniform_bucket_level_access"] = uniform_bucket_level_access
    if force_destroy != None:
        body["force_destroy"] = force_destroy
    if lifecycle_rule != None:
        body["lifecycle_rule"] = lifecycle_rule
    if labels != None:
        body["labels"] = labels
    if public_access_prevention != None:
        body["public_access_prevention"] = public_access_prevention
    if versioning != None:
        body["versioning"] = versioning if type(versioning) == type([]) else [versioning]
    return resource(
        rtype = "google_storage_bucket",
        name = name,
        body = body,
        attrs = ["id", "name", "url", "self_link"],
    )

# ---------- IAM ------------------------------------------------------------

def project_iam_member(name, project, role, member, condition = None):
    """`google_project_iam_member` — non-authoritative single-member binding.

    Use this (not `_binding` or `_policy`) so adding a role here doesn't
    evict members granted out-of-band.
    """
    body = {
        "project": project,
        "role": role,
        "member": member,
    }
    if condition != None:
        body["condition"] = condition if type(condition) == type([]) else [condition]
    return resource(
        rtype = "google_project_iam_member",
        name = name,
        body = body,
        attrs = ["id", "etag"],
    )

def service_account_iam_member(name, service_account_id, role, member, condition = None):
    """`google_service_account_iam_member` — non-authoritative."""
    body = {
        "service_account_id": service_account_id,
        "role": role,
        "member": member,
    }
    if condition != None:
        body["condition"] = condition if type(condition) == type([]) else [condition]
    return resource(
        rtype = "google_service_account_iam_member",
        name = name,
        body = body,
        attrs = ["id", "etag"],
    )

def storage_bucket_iam_member(name, bucket, role, member, condition = None):
    """`google_storage_bucket_iam_member` — non-authoritative."""
    body = {
        "bucket": bucket,
        "role": role,
        "member": member,
    }
    if condition != None:
        body["condition"] = condition if type(condition) == type([]) else [condition]
    return resource(
        rtype = "google_storage_bucket_iam_member",
        name = name,
        body = body,
        attrs = ["id", "etag"],
    )

# ---------- workload identity federation -----------------------------------

def iam_workload_identity_pool(
        name,
        project,
        workload_identity_pool_id,
        display_name = None,
        description = None,
        disabled = None):
    """`google_iam_workload_identity_pool`."""
    body = {
        "project": project,
        "workload_identity_pool_id": workload_identity_pool_id,
    }
    if display_name != None:
        body["display_name"] = display_name
    if description != None:
        body["description"] = description
    if disabled != None:
        body["disabled"] = disabled
    return resource(
        rtype = "google_iam_workload_identity_pool",
        name = name,
        body = body,
        attrs = ["id", "name", "state", "workload_identity_pool_id"],
    )

def iam_workload_identity_pool_provider(
        name,
        project,
        workload_identity_pool_id,
        workload_identity_pool_provider_id,
        display_name = None,
        description = None,
        disabled = None,
        attribute_mapping = None,
        attribute_condition = None,
        oidc = None,
        aws = None,
        saml = None,
        x509 = None):
    """`google_iam_workload_identity_pool_provider`.

    Exactly one of `oidc` / `aws` / `saml` / `x509` must be set, per the
    Google provider's schema. Each accepts either a dict (single block,
    we wrap) or a list of one (already the JSON block-as-array shape).
    """
    body = {
        "project": project,
        "workload_identity_pool_id": workload_identity_pool_id,
        "workload_identity_pool_provider_id": workload_identity_pool_provider_id,
    }
    if display_name != None:
        body["display_name"] = display_name
    if description != None:
        body["description"] = description
    if disabled != None:
        body["disabled"] = disabled
    if attribute_mapping != None:
        body["attribute_mapping"] = attribute_mapping
    if attribute_condition != None:
        body["attribute_condition"] = attribute_condition
    if oidc != None:
        body["oidc"] = oidc if type(oidc) == type([]) else [oidc]
    if aws != None:
        body["aws"] = aws if type(aws) == type([]) else [aws]
    if saml != None:
        body["saml"] = saml if type(saml) == type([]) else [saml]
    if x509 != None:
        body["x509"] = x509 if type(x509) == type([]) else [x509]
    return resource(
        rtype = "google_iam_workload_identity_pool_provider",
        name = name,
        body = body,
        attrs = ["id", "name", "state"],
    )

# ---------- monitoring -----------------------------------------------------

def monitoring_notification_channel(name, project, display_name, type, labels):
    """`google_monitoring_notification_channel`.

    `type` is one of "email", "slack", "pagerduty", "webhook_tokenauth", etc.
    `labels` is the type-specific config dict (e.g. {"email_address": "..."}).
    """
    return resource(
        rtype = "google_monitoring_notification_channel",
        name = name,
        body = {
            "project": project,
            "display_name": display_name,
            "type": type,
            "labels": labels,
        },
        attrs = ["id", "name"],
    )

def logging_metric(name, project, metric_name, filter, description = None):
    """`google_logging_metric` — counter metric over matching log entries.

    Defaults to `DELTA` / `INT64` (count of entries per minute). Suitable
    for rate-based or absence-based alerts. Bucket / labels / value
    extractor are not exposed yet — add when needed.
    """
    body = {
        "project": project,
        "name": metric_name,
        "filter": filter,
        "metric_descriptor": {
            "metric_kind": "DELTA",
            "value_type": "INT64",
            "unit": "1",
        },
    }
    if description != None:
        body["description"] = description
    return resource(
        rtype = "google_logging_metric",
        name = name,
        body = body,
        attrs = ["id", "name"],
    )

def monitoring_alert_policy_metric_absent(
        name,
        project,
        display_name,
        metric_filter,
        notification_channels,
        severity,
        documentation = None,
        duration = "3600s"):
    """`google_monitoring_alert_policy` for a metric-absence condition.

    Fires when `metric_filter` reports no data points for `duration`.
    Use to alert on log streams that should always have *some* volume —
    if they go silent, either the source stopped or the audit config
    was disabled.

    `metric_filter` is a Cloud Monitoring filter (not a logging filter),
    typically `metric.type="logging.googleapis.com/user/<metric_name>"`.

    `severity` is one of "CRITICAL", "ERROR", "WARNING". Required:
    GCP renders an empty severity in notifications, which makes triage
    by inbox harder than it needs to be.
    """
    body = {
        "project": project,
        "display_name": display_name,
        "combiner": "OR",
        "severity": severity,
        "conditions": [{
            "display_name": display_name,
            "condition_absent": [{
                "filter": metric_filter,
                "duration": duration,
                "aggregations": [{
                    "alignment_period": "300s",
                    "per_series_aligner": "ALIGN_RATE",
                }],
                "trigger": [{"count": 1}],
            }],
        }],
        "notification_channels": notification_channels,
    }
    if documentation != None:
        body["documentation"] = [{
            "content": documentation,
            "mime_type": "text/markdown",
        }]
    return resource(
        rtype = "google_monitoring_alert_policy",
        name = name,
        body = body,
        attrs = ["id", "name"],
    )

def monitoring_alert_policy_log_match(
        name,
        project,
        display_name,
        filter,
        notification_channels,
        severity,
        documentation = None,
        rate_limit_period = "300s"):
    """`google_monitoring_alert_policy` for a log-match condition.

    `condition_matched_log` fires once per log entry matching `filter`,
    with a per-channel rate-limit window (`rate_limit_period`) so a
    bursty pattern doesn't page on every entry.

    `documentation` is the runbook text the notification carries — make
    it useful at 3am, not a description of the filter.

    `severity` is one of "CRITICAL", "ERROR", "WARNING". Required:
    GCP renders an empty severity in notifications, which makes triage
    by inbox harder than it needs to be.
    """
    body = {
        "project": project,
        "display_name": display_name,
        "combiner": "OR",
        "severity": severity,
        "conditions": [{
            "display_name": display_name,
            "condition_matched_log": [{"filter": filter}],
        }],
        "alert_strategy": [{
            "notification_rate_limit": [{"period": rate_limit_period}],
            # Auto-close the incident after 30 min of no matches —
            # log-match incidents don't auto-close otherwise.
            "auto_close": "1800s",
        }],
        "notification_channels": notification_channels,
    }
    if documentation != None:
        body["documentation"] = [{
            "content": documentation,
            "mime_type": "text/markdown",
        }]
    return resource(
        rtype = "google_monitoring_alert_policy",
        name = name,
        body = body,
        attrs = ["id", "name"],
    )
