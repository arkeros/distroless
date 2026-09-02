"""Single source of truth for the mirror's verification policy.

Two signers put artifacts on a mirror digest, and consumers must pin both:

  * Signature and SBOM attestation — signed by *our* workflow. Consumers pin
    `CERTIFICATE_IDENTITY_REGEXP` + `CERTIFICATE_OIDC_ISSUER`.
  * Platform provenance — signed by slsa-github-generator's reusable workflow,
    whose certificate is the same for every project using it. Consumers pin
    `PROVENANCE_BUILDER_ID` and check `PROVENANCE_SOURCE_URI` *inside* the
    predicate with `slsa-verifier verify-image`. See ADR 0014.

CI's verify steps, the negative tests and the consumer-facing docs all derive
from the values below; renaming the workflow file or migrating the signing
branch is an edit to this file (and to `web/internal/policy`, which the
Directory duplicates because Go cannot load Starlark).
"""

# Source repo. The "github.com" host is implied; change-control this only
# if the project moves runners (Codeberg, etc.).
SOURCE_REPO = "github.com/arkeros/distroless"

# The single workflow file authorized to sign mirror images. CODEOWNERS
# on this file is the human-review gate; the OIDC subject is the
# cosign-verifier perimeter.
WORKFLOW_PATH = ".github/workflows/ci.yaml"

# Git ref pinned in OIDC certificate `subject` claims. `main`-only — the
# mirror publishes on every push to main; tags here are calendar
# checkpoints, not release events (ADR 0006, kept by ADR 0014).
WORKFLOW_REF = "refs/heads/main"

# OIDC issuer. GitHub Actions for both signers.
CERTIFICATE_OIDC_ISSUER = "https://token.actions.githubusercontent.com"

# The reusable workflow that writes platform provenance. `slsa-verifier
# --builder-id` takes it without the `@ref`; the tag is checked separately
# against the `v[0-9]+.[0-9]+.[0-9]+` shape the generator requires.
PROVENANCE_BUILDER_ID = "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/generator_container_slsa3.yml"

# What `slsa-verifier --source-uri` must find in the predicate. The
# generator's certificate names the generator, not us; this is the only
# thing that binds a provenance statement to *this* repository.
PROVENANCE_SOURCE_URI = SOURCE_REPO

# --- Derived ---

def _regex_escape(s):
    return s.replace(".", "\\.")

# Consumer side: regex consumers pin via `cosign verify` /
# `cosign verify-attestation --certificate-identity-regexp=...`.
# Workflow-bound (matches only the specific WORKFLOW_PATH on WORKFLOW_REF),
# not repo-bound — adding a new workflow file is not enough to mint a valid
# mirror signature.
CERTIFICATE_IDENTITY_REGEXP = "^https://{repo}/{path}@{ref}$".format(
    repo = _regex_escape(SOURCE_REPO),
    path = _regex_escape(WORKFLOW_PATH),
    ref = _regex_escape(WORKFLOW_REF),
)
