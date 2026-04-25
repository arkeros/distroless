#!/usr/bin/env bash
# Walk N executables in the order given. Stops on the first failure.
#
# Args: one runfiles path per binary, in topological order.
set -euo pipefail

for binary in "$@"; do
  echo "==> $binary"
  "$PWD/$binary"
done
