#!/usr/bin/env bash
# Asserts the root re-export shim (//:cosign.bzl) covers exactly the public
# symbols defined in //cosign:defs.bzl + //cosign/toolchain:toolchain.bzl.
#
# Without this, adding a new rule to defs.bzl (or toolchain.bzl) and
# forgetting to re-export from the root shim leaves consumers using the
# short-form `load("@cosign.bzl", ...)` unable to see the new rule —
# silent drift that surfaces only on first attempted use.
#
# Implementation is grep-based: we extract the *re-exported names* on the
# left side of `name = _name` assignments. Both files use the same
# `_X = X; X = _X` private-load + public-re-export pattern.

set -o pipefail -o errexit -o nounset

if [[ -n "${TEST_SRCDIR:-}" ]]; then
    # Bazel test invocation. The cosign.bzl module's runfiles live under
    # ${TEST_SRCDIR}/cosign.bzl+/ when consumed via bzlmod (canonical repo
    # name with the `+` suffix). When the test runs from inside the module
    # itself, bzlmod still uses the canonical name. Glob to be tolerant of
    # name-mangling differences across Bazel versions.
    MODULE_DIR=$(find "${TEST_SRCDIR}" -maxdepth 1 -type d -name 'cosign.bzl*' | head -1)
    if [[ -z "${MODULE_DIR}" ]]; then
        echo "ERROR: could not locate cosign.bzl module runfiles dir under ${TEST_SRCDIR}" >&2
        ls "${TEST_SRCDIR}" >&2
        exit 1
    fi
    ROOT_FILE="${MODULE_DIR}/cosign.bzl"
    DEFS_FILE="${MODULE_DIR}/cosign/defs.bzl"
    TOOLCHAIN_FILE="${MODULE_DIR}/cosign/toolchain/toolchain.bzl"
else
    # Direct invocation (e.g. from a dev shell) — assume CWD is the module root.
    ROOT_FILE="cosign.bzl"
    DEFS_FILE="cosign/defs.bzl"
    TOOLCHAIN_FILE="cosign/toolchain/toolchain.bzl"
fi

extract_reexports() {
    # Lines like `name = _name` (whitespace-permissive). Print just the LHS name.
    grep -E '^[a-z_][a-z_0-9]* = _[a-z_][a-z_0-9]*$' "$1" | awk '{print $1}'
}

extract_rule_defs() {
    # Top-level `name = rule(` definitions. Print the name.
    grep -E '^[a-z_][a-z_0-9]* = rule\(' "$1" | awk '{print $1}'
}

WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

extract_reexports "${ROOT_FILE}" | sort > "${WORK}/root.txt"

# defs.bzl re-exports rules from cosign/private/*.bzl.
extract_reexports "${DEFS_FILE}" > "${WORK}/expected.txt"

# toolchain.bzl defines `cosign_toolchain` directly as a rule (no re-export
# pattern there since it's all in one file). Pick it up from the rule def.
extract_rule_defs "${TOOLCHAIN_FILE}" >> "${WORK}/expected.txt"

sort -o "${WORK}/expected.txt" "${WORK}/expected.txt"

if ! diff -u "${WORK}/expected.txt" "${WORK}/root.txt"; then
    cat >&2 <<EOF

ERROR: bazel/modules/cosign.bzl/cosign.bzl (root re-export shim) is out of
sync with the public API in //cosign:defs.bzl + //cosign/toolchain:toolchain.bzl.

The diff above shows symbols that should be re-exported from the root shim
but aren't (or are re-exported but no longer exist). Update cosign.bzl to
match: every public rule in defs.bzl/toolchain.bzl needs a corresponding
'name = _name' line in the root shim so consumers using

    load("@cosign.bzl", "name")

can see it.
EOF
    exit 1
fi

echo "OK: cosign.bzl re-export shim covers $(wc -l < "${WORK}/root.txt" | tr -d ' ') public symbol(s)."
