#!/usr/bin/env bash
# Runs a `crane_tag` target against a fake crane and asserts the exact
# commands it issues. The fake is selected with CRANE, the same override a
# developer can use; nothing here touches a registry.
set -o errexit -o nounset -o pipefail

WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

cat > "${WORK}/crane" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${CRANE_LOG}"
EOF
chmod +x "${WORK}/crane"

CRANE="${WORK}/crane" CRANE_LOG="${WORK}/log" "${TAG_BIN}"

if ! diff -u "${EXPECTED}" "${WORK}/log"; then
    echo "ERROR: crane_tag issued different commands than expected (see diff above)." >&2
    exit 1
fi
echo "OK: $(wc -l < "${WORK}/log" | tr -d ' ') tag command(s) as expected."
