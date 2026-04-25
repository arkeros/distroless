#!/usr/bin/env bash
# Push the registry image to GAR (deploy-side) and GHCR (public mirror),
# materialize the just-built image digest as a Terraform auto.tfvars.json,
# then terraform apply. Digest flows from Bazel → Terraform with no manual
# tfvars edit.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

cd "$REPO_ROOT"
# --stamp is required for the template-substituted tags (e.g.
# `{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}`) to resolve against the
# workspace-status script (bazel/workspace_status.sh). Without it they
# collapse to `<no value>` and the registry rejects the tag.
bazel run --stamp //oci/cmd/registry:image_push_gar
bazel run --stamp //oci/cmd/registry:image_tfvars

cd "$SCRIPT_DIR/terraform"
terraform init
terraform apply
