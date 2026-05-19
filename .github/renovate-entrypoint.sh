#!/bin/bash

# Install required tools using containerbase
install-tool node v24.13.0
install-tool pnpm v10.12.3
install-tool bazelisk v1.27.0
ln -sf "$(which bazelisk)" /usr/local/bin/bazel

# Renovate's `terraform-lockfile` manager invokes `terraform providers lock`
# directly to refresh `.terraform.lock.hcl` after a provider bump. The
# version here just needs to be recent enough to understand the lockfile
# format we commit; the workspace's bazel-pinned terraform (a separate
# install via the bazel toolchain) is what `aspect plan` / `aspect apply`
# actually use.
install-tool terraform v1.14.8

renovate
