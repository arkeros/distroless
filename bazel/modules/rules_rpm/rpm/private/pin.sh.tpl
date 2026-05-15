#!/usr/bin/env bash
# Wrapper around the `pin` Go binary. Runs under `bazel run`, so
# BUILD_WORKSPACE_DIRECTORY is the consumer's source tree.
set -euo pipefail

cd "${BUILD_WORKSPACE_DIRECTORY:?must be invoked via bazel run}"

exec "{TOOL}" \
    --repo-url "{REPO_URL}" \
    --gpg-key "{GPG_KEY}" \
    --packages "{PACKAGES}" \
    --architectures "{ARCHITECTURES}" \
    --lock-out "{LOCK_FILE}" \
    "$@"
