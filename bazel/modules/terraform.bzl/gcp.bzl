"""Re-export shim for `load("@terraform.bzl//:gcp.bzl", ...)`.

GCP resource constructors. Each returns a struct compatible with
`tf_root(docs=...)`; see `terraform/resources/gcp.bzl` for the
implementations and `tf_root` itself for how `docs` is consumed.
"""

load(
    "//terraform/resources:gcp.bzl",
    _artifact_registry_repository = "artifact_registry_repository",
    _ephemeral_google_secret_manager_secret_version = "ephemeral_google_secret_manager_secret_version",
    _google_cloud_run_v2_job = "google_cloud_run_v2_job",
    _google_cloud_run_v2_job_iam_member = "google_cloud_run_v2_job_iam_member",
    _google_cloud_run_v2_service = "google_cloud_run_v2_service",
    _google_cloud_run_v2_service_iam_member = "google_cloud_run_v2_service_iam_member",
    _google_cloud_scheduler_job = "google_cloud_scheduler_job",
    _google_provider = "google_provider",
    _iam_workload_identity_pool = "iam_workload_identity_pool",
    _iam_workload_identity_pool_provider = "iam_workload_identity_pool_provider",
    _logging_metric = "logging_metric",
    _logging_project_exclusion = "logging_project_exclusion",
    _monitoring_alert_policy_log_match = "monitoring_alert_policy_log_match",
    _monitoring_alert_policy_metric_absent = "monitoring_alert_policy_metric_absent",
    _monitoring_notification_channel = "monitoring_notification_channel",
    _project_iam_audit_config = "project_iam_audit_config",
    _project_iam_member = "project_iam_member",
    _project_service = "project_service",
    _secret_manager_secret = "secret_manager_secret",
    _secret_manager_secret_iam_member = "secret_manager_secret_iam_member",
    _service_account = "service_account",
    _service_account_iam_member = "service_account_iam_member",
    _sql_database_instance = "sql_database_instance",
    _storage_bucket = "storage_bucket",
    _storage_bucket_iam_member = "storage_bucket_iam_member",
)

google_provider = _google_provider
project_service = _project_service
project_iam_audit_config = _project_iam_audit_config
project_iam_member = _project_iam_member
logging_project_exclusion = _logging_project_exclusion
logging_metric = _logging_metric
service_account = _service_account
service_account_iam_member = _service_account_iam_member
google_cloud_run_v2_service = _google_cloud_run_v2_service
google_cloud_run_v2_service_iam_member = _google_cloud_run_v2_service_iam_member
google_cloud_run_v2_job = _google_cloud_run_v2_job
google_cloud_run_v2_job_iam_member = _google_cloud_run_v2_job_iam_member
google_cloud_scheduler_job = _google_cloud_scheduler_job
secret_manager_secret = _secret_manager_secret
secret_manager_secret_iam_member = _secret_manager_secret_iam_member
ephemeral_google_secret_manager_secret_version = _ephemeral_google_secret_manager_secret_version
sql_database_instance = _sql_database_instance
artifact_registry_repository = _artifact_registry_repository
storage_bucket = _storage_bucket
storage_bucket_iam_member = _storage_bucket_iam_member
iam_workload_identity_pool = _iam_workload_identity_pool
iam_workload_identity_pool_provider = _iam_workload_identity_pool_provider
monitoring_notification_channel = _monitoring_notification_channel
monitoring_alert_policy_metric_absent = _monitoring_alert_policy_metric_absent
monitoring_alert_policy_log_match = _monitoring_alert_policy_log_match
