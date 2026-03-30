#!/bin/bash

# Install required tools using containerbase
install-tool node v24.13.0
install-tool pnpm v10.12.3
install-tool bazelisk v1.27.0
ln -sf "$(which bazelisk)" /usr/local/bin/bazel

renovate
