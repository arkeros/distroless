load(
    "//devtools/bifrost/terraform/modules/service_kubernetes:defs.bzl",
    "service_kubernetes",
)
load("//devtools/build/tools/tf:defs.bzl", "tf_root")
load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "google_provider",
    "secret_manager_secret",
    "secret_manager_secret_iam_member",
    "sql_database_instance",
)
load("//devtools/build/tools/tf/resources:k8s.bzl", "kubernetes_job_v1")

# Composition smoke target — never applied, never planned. Demonstrates
# wiring service_kubernetes to sibling resources: a Cloud SQL instance
# whose private IP feeds the service's env, a Secret Manager secret
# holding the DB password, and a migration Job that must complete before
# the Deployment rolls out.
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
        }],
    },
)

db_password = secret_manager_secret(
    name = "db_password",
    replication = {"auto": [{}]},
    secret_id = "api-db-password",
)

# Migration Job — typed (not SSA) so terraform tracks completion via the
# K8s API. Name with the image digest if you want per-version migrations.
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
        "min": 1,
    },
    # Pin Deployment ordering to the migration Job. Without this, terraform
    # plans the two in either order and pods may roll out with a stale
    # schema. service_kubernetes appends this to its internal Deployment
    # depends_on list (alongside the SA and env Secret).
    depends_on = [migrate.addr],
    # DB hostname wired through Starlark — `db.private_ip_address` is the
    # interpolation `${google_sql_database_instance.api.private_ip_address}`.
    env = {
        "DB_HOST": db.private_ip_address,
        "DB_NAME": "api",
    },
    image = "europe-docker.pkg.dev/senku-prod/api/api@sha256:" + ("1" * 64),
    namespace = NAMESPACE,
    port = 8080,
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

# Grant the runtime GSA access to the DB password secret. service_kubernetes
# returns `service_account_email`, which slots straight into a
# `member = "serviceAccount:..."` reference.
secret_grant = secret_manager_secret_iam_member(
    name = "api_db_password",
    member = "serviceAccount:{}".format(api.service_account_email),
    role = "roles/secretmanager.secretAccessor",
    secret_id = db_password.id,
)

tf_root(
    name = "terraform",
    docs = [
        google_provider(
            project = PROJECT,
            region = REGION,
        ),
        db,
        db_password,
        migrate,
        api,
        secret_grant,
    ],
    required_providers = {
        "google": {
            "source": "hashicorp/google",
            "version": "~> 7.0",
        },
        "kubernetes": {
            "source": "hashicorp/kubernetes",
            "version": "~> 2.38",
        },
    },
    visibility = ["//visibility:private"],
)
