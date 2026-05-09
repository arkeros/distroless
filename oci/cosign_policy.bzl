"""Single source of truth for the cosign signing policy.

Producer side (`mirror_push`) bakes `BUILDER_ID` into the SLSA predicate's
`runDetails.builder.id`. Consumer side (CI verify steps + external
`cosign verify-attestation` invocations) pin `CERTIFICATE_IDENTITY_REGEXP`
to validate the signature's OIDC subject. Both must agree, which is why
they're derived from the same constants here. See ADR 0006.

Renaming the workflow file or migrating the signing branch only requires
editing this file; the producer rule, the consumer regex, the CI policy
env file (`:cosign_policy_env`), and the consumer-facing docs all derive
from the values below.
"""

# Source repo. The "github.com" host is implied; change-control this only
# if the project moves runners (Codeberg, etc.) — see ADR 0006's note on
# verify-policy lock-in for runner migration.
SOURCE_REPO = "github.com/arkeros/senku"

# The single workflow file authorized to sign mirror images. CODEOWNERS
# on this file is the human-review gate; the OIDC subject is the
# Bazel-built / cosign-verifier perimeter.
WORKFLOW_PATH = ".github/workflows/ci.yaml"

# Git ref pinned in OIDC certificate `subject` claims. `main`-only —
# tags here are calendar checkpoints, not release events (ADR 0006).
WORKFLOW_REF = "refs/heads/main"

# OIDC issuer. Today only GitHub Actions; if migrating runners
# (BuildBuddy, Tekton, Cloud Build, Codeberg) this would gain alternates
# and consumer policies would need updating in coordination.
CERTIFICATE_OIDC_ISSUER = "https://token.actions.githubusercontent.com"

# --- Derived ---

# Producer side: baked into the SLSA predicate's `runDetails.builder.id`.
BUILDER_ID = "https://{repo}/{path}@{ref}".format(
    repo = SOURCE_REPO,
    path = WORKFLOW_PATH,
    ref = WORKFLOW_REF,
)

# Stable URI describing the schema of the SLSA predicate's `buildType`.
# Consumers treat it as opaque, but it MUST be stable across releases —
# changing it breaks consumer policies that pin on `predicateType`.
BUILD_TYPE_URI = "https://{repo}/cosign.bzl/v1/bazel-mirror".format(
    repo = SOURCE_REPO,
)

def _regex_escape(s):
    return s.replace(".", "\\.")

# Consumer side: regex consumers pin via `cosign verify-attestation
# --certificate-identity-regexp=...`. Workflow-bound (matches only the
# specific WORKFLOW_PATH on WORKFLOW_REF), not repo-bound — adding a
# new workflow file is not enough to mint a valid mirror signature.
CERTIFICATE_IDENTITY_REGEXP = "^https://{repo}/{path}@{ref}$".format(
    repo = _regex_escape(SOURCE_REPO),
    path = _regex_escape(WORKFLOW_PATH),
    ref = _regex_escape(WORKFLOW_REF),
)
