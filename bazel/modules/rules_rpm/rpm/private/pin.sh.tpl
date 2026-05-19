#!/usr/bin/env bash
# Wrapper around the `pin` Go binary. Runs under `bazel run`, so
# BUILD_WORKSPACE_DIRECTORY is the consumer's source tree.
set -euo pipefail

# {TOOL} and {GPG_KEY} are runfiles-relative paths; resolve both to
# absolute *before* cd so the exec below survives the working-directory
# switch into the consumer's source tree. (The signature check now
# actually reads the key — when it was a no-op, leaving GPG_KEY
# unresolved was harmless. It is no longer.)
TOOL_ABS="$(cd "$(dirname "{TOOL}")" && pwd)/$(basename "{TOOL}")"
GPG_KEY_ABS="$(cd "$(dirname "{GPG_KEY}")" && pwd)/$(basename "{GPG_KEY}")"

cd "${BUILD_WORKSPACE_DIRECTORY:?must be invoked via bazel run}"

exec "$TOOL_ABS" \
    --repo-url "{REPO_URL}" \
    --gpg-key "$GPG_KEY_ABS" \
    --packages "{PACKAGES}" \
    --architectures "{ARCHITECTURES}" \
    --lock-out "{LOCK_FILE}" \
    --repomd-signature "{REPOMD_SIGNATURE}" \
    "$@"
