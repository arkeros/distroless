load("//devtools/bifrost/modules:kubernetes.bzl", "service_kubernetes")
load("//devtools/build/tools/tf:defs.bzl", "tf_root")
load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "google_provider",
    "secret_manager_secret",
    "secret_manager_secret_iam_member",
    "sql_database_instance",
)
load(
    "//devtools/build/tools/tf/resources:k8s.bzl",
    "kubernetes_job_v1",
    "kubernetes_provider",
)

# Composition smoke target — never planned, never applied, just analyzed.
# Demonstrates the three integration idioms that don't fit in a basic
# example: cross-resource env wiring (DB hostname → service env), a
# sibling depends_on (migration Job → Deployment), and an IAM grant on
# the runtime GSA (DB password access).
#
# The `kubernetes_provider` block below carries placeholder strings only
# so the smoke target is self-contained. Real roots wire it from data
# sources — typically `google_container_cluster` for host +
# cluster_ca_certificate and `google_client_config` for a short-lived
# access token — never literal strings (a baked-in token would land in
# terraform state).
PROJECT = "senku-prod"

REGION = "europe-west1"

NAMESPACE = "default"

db = sql_database_instance(
    name = "api",
    database_version = "POSTGRES_16",
    deletion_protection = True,
    instance_name = "api-db",
    region = REGION,
    settings = {
        "tier": "db-f1-micro",
        "ip_configuration": [{
            "ipv4_enabled": False,
            "private_network": "projects/{}/global/networks/default".format(PROJECT),
            "ssl_mode": "ENCRYPTED_ONLY",
        }],
        "backup_configuration": [{
            "enabled": True,
            "point_in_time_recovery_enabled": True,
            "start_time": "03:00",
        }],
        "database_flags": [{
            "name": "cloudsql.enable_pgaudit",
            "value": "on",
        }],
    },
)

db_password = secret_manager_secret(
    name = "db_password",
    replication = {"auto": [{}]},
    secret_id = "api-db-password",
)

# Migration Job — typed (not SSA) so terraform tracks completion via the
# K8s API. wait_for_completion = True blocks the apply on this Job
# finishing before the Deployment rolls out.
migrate = kubernetes_job_v1(
    name = "migrate",
    metadata = {
        "name": "api-migrate",
        "namespace": NAMESPACE,
    },
    spec = {
        "backoff_limit": 0,
        "template": [{
            "metadata": [{"labels": {"app.kubernetes.io/name": "api-migrate"}}],
            "spec": [{
                "restart_policy": "Never",
                "container": [{
                    "name": "migrate",
                    "image": "europe-docker.pkg.dev/senku-prod/api/migrate@sha256:" + ("0" * 64),
                    "env": [{
                        "name": "DB_HOST",
                        "value": db.private_ip_address,
                    }],
                }],
            }],
        }],
    },
    wait_for_completion = True,
)

api = service_kubernetes(
    name = "api",
    autoscaling = {
        "max": 5,
        "min": 2,
    },
    depends_on = [migrate.addr],
    env = {
        "DB_HOST": db.private_ip_address,
        "DB_NAME": "api",
    },
    image = "europe-docker.pkg.dev/senku-prod/api/api@sha256:" + ("1" * 64),
    namespace = NAMESPACE,
    probes = {
        "liveness_path": "/healthz",
        "readiness_path": "/ready",
        "startup_path": "/healthz",
    },
    project = PROJECT,
    resources = {
        "cpu": 0.25,
        "memory": 512,
    },
    secret_env = {
        "DB_PASSWORD": {
            "project": PROJECT,
            "secret": db_password.secret_id,
            "version": "3",
        },
    },
)

tf_root(
    name = "terraform",
    docs = [
        google_provider(
            project = PROJECT,
            region = REGION,
        ),
        kubernetes_provider(
            cluster_ca_certificate = "REPLACE_FROM_CLUSTER_HARNESS",
            host = "https://kubernetes.example.invalid",
            token = "REPLACE_FROM_CLUSTER_HARNESS",
        ),
        db,
        db_password,
        migrate,
        api,
        secret_manager_secret_iam_member(
            name = "api_db_password",
            member = "serviceAccount:{}".format(api.service_account_email),
            role = "roles/secretmanager.secretAccessor",
            secret_id = db_password.id,
        ),
    ],
    providers = [
        "@terraform_providers//:google",
        "@terraform_providers//:kubernetes",
    ],
    visibility = ["//visibility:private"],
)
