"""Identity, derived strings, and resource declarations for the Artifact
Registry repo this root provisions.

Single source of truth: the GCP-level primitives (project, location,
repository_id) live here, the Bazel-facing strings (`GAR_REGISTRY`,
`GAR_REPOSITORY_PREFIX`) are derived from them, and the Terraform resources
that actually create the thing are constructed from the same constants.
Image-push rules and anything else constructing GAR URIs load the strings
from here so they always agree with what this root deploys.
"""

load(
    "//devtools/build/tools/tf/resources:gcp.bzl",
    "artifact_registry_repository",
    "project_service",
)

GAR_PROJECT = "senku-prod"

# `europe` (not `us`) because the team and most traffic are EU-centric.
#
# Multi-region vs. per-Cloud-Run-region fan-out: regional fan-out buys at
# most ~1s of cold-start pull latency for far-from-EU Cloud Run regions, at
# the cost of making every release a 5-way push-consistency problem. Cold
# starts aren't in the request-path SLO, so not worth it. If cold-start
# latency ever needs to drop to zero for a specific service, raise its
# Cloud Run `scaling.min` instead — that kills the cold-start class
# entirely rather than shaving a second off it.
GAR_LOCATION = "europe"

GAR_REPOSITORY_ID = "containers"

# Derived: the multi-region hostname AR exposes (`<location>-docker.pkg.dev`).
GAR_REGISTRY = GAR_LOCATION + "-docker.pkg.dev"

# Derived: `<project>/<repository_id>` — what image_push rules prepend before
# the image name (e.g. `<repository_prefix>/registry`).
GAR_REPOSITORY_PREFIX = GAR_PROJECT + "/" + GAR_REPOSITORY_ID

# Artifact Registry API has to be enabled before we can create repositories
# in the project. Managed here so a fresh project bootstraps in a single
# apply instead of requiring an out-of-band `gcloud services enable`.
#
# `disable_on_destroy = False` leaves the API enabled even if this root is
# destroyed — disabling an API on a project in active use is never what we
# want.
ARTIFACT_REGISTRY_API = project_service(
    name = "artifactregistry",
    project = GAR_PROJECT,
    service = "artifactregistry.googleapis.com",
    disable_on_destroy = False,
)

# Single multi-region repo. All Senku-built images live here; consumers
# (Cloud Run in any region, K8s clusters, local `docker pull`) pull from
# `<GAR_REGISTRY>/<GAR_REPOSITORY_PREFIX>/...`.
CONTAINERS_REPO = artifact_registry_repository(
    name = GAR_REPOSITORY_ID,
    project = GAR_PROJECT,
    location = GAR_LOCATION,
    repository_id = GAR_REPOSITORY_ID,
    format = "DOCKER",
    description = "Private container images for Senku workloads (deploy-time pulls by Cloud Run, K8s, etc.).",
    depends_on = [ARTIFACT_REGISTRY_API.addr],
)
