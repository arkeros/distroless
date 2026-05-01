"""OpenVEX 0.2.0 document authoring.

`vex_document` emits a JSON file consumable by tools that accept OpenVEX
(notably grype's `--vex` flag). Statements are built with `vex_statement`,
which validates status/justification combinations at loading phase so
malformed documents fail the build before they reach a scanner.

NOTE: vex_statement requires an `expires` field (RFC3339 date) that is a
**senku-flavored extension** to the OpenVEX 0.2.0 spec — the spec has no
native expiry mechanism. The field appears in the emitted JSON; OpenVEX
consumers that don't recognize it (every consumer except this repo's
grype.bzl integration today) will silently skip it. grype.bzl validates
the date action-side: must be present, not past, and ≤90 days in the
future. The mechanism forces periodic re-justification — silencing
decisions are debts with due dates, not permanent settlements.

Example:
    load("//oci:vex.bzl", "vex_document", "vex_statement")

    vex_document(
        name = "registry_vex",
        id = "https://github.com/arkeros/senku/blob/main/oci/cmd/registry/BUILD",
        author = "rafael@arquero.cat",
        timestamp = "2026-04-30T00:00:00Z",
        statements = [
            vex_statement(
                vulnerability = "CVE-2026-4046",
                products = ["pkg:oci/registry"],
                status = "not_affected",
                justification = "vulnerable_code_not_in_execute_path",
                impact_statement = "registry never feeds untrusted input through iconv() with IBM1390/IBM1399",
                expires = "2026-07-29",  # ≤90d from authoring; re-justify or delete
            ),
        ],
    )
"""

_CONTEXT = "https://openvex.dev/ns/v0.2.0"

_STATUSES = [
    "not_affected",
    "affected",
    "fixed",
    "under_investigation",
]

# https://github.com/openvex/spec/blob/main/OPENVEX-SPEC.md#status-justifications
_JUSTIFICATIONS = [
    "component_not_present",
    "vulnerable_code_not_present",
    "vulnerable_code_not_in_execute_path",
    "vulnerable_code_cannot_be_controlled_by_adversary",
    "inline_mitigations_already_exist",
]

def _validate_expires_format(expires):
    """Loading-phase shape check for an RFC3339 date (YYYY-MM-DD).

    Action-time validation (not in past, not >90d in future) lives in
    grype.bzl's test runner — Starlark has no datetime access without
    breaking hermeticity.
    """
    if type(expires) != "string" or len(expires) != 10:
        fail("vex_statement: expires=%r must be RFC3339 date YYYY-MM-DD" % expires)
    if expires[4] != "-" or expires[7] != "-":
        fail("vex_statement: expires=%r must be RFC3339 date YYYY-MM-DD" % expires)
    for i in [0, 1, 2, 3, 5, 6, 8, 9]:
        if not expires[i].isdigit():
            fail("vex_statement: expires=%r must be RFC3339 date YYYY-MM-DD" % expires)

def vex_statement(
        vulnerability,
        products,
        status,
        expires,
        justification = None,
        impact_statement = None,
        action_statement = None):
    """Build one VEX statement. Validates field combinations at loading phase.

    Args:
      vulnerability: CVE ID (string), e.g. "CVE-2026-4046".
      products: List of product identifiers, ideally PURLs
        (e.g. "pkg:oci/registry" or "pkg:oci/registry@sha256:...").
      status: One of "not_affected", "affected", "fixed", "under_investigation".
      expires: RFC3339 date "YYYY-MM-DD" — review-by deadline. The build
        fails when this date is past or more than 90 days in the future
        (see grype.bzl test runner). Forces re-justification rather than
        permanent silencing — bump the date or delete the statement.
      justification: Required when status == "not_affected". One of
        component_not_present, vulnerable_code_not_present,
        vulnerable_code_not_in_execute_path,
        vulnerable_code_cannot_be_controlled_by_adversary,
        inline_mitigations_already_exist.
      impact_statement: Free-text rationale. Optional but strongly recommended
        for not_affected so the audit trail explains why.
      action_statement: Required when status == "affected". Mitigation /
        workaround text.

    Returns:
      A dict consumable by vex_document.
    """
    if status not in _STATUSES:
        fail("vex_statement: status=%r must be one of %s" % (status, _STATUSES))
    if status == "not_affected":
        if not justification:
            fail("vex_statement: status=not_affected requires a justification")
        if justification not in _JUSTIFICATIONS:
            fail("vex_statement: justification=%r must be one of %s" % (justification, _JUSTIFICATIONS))
    if status == "affected" and not action_statement:
        fail("vex_statement: status=affected requires an action_statement")
    if not products:
        fail("vex_statement: products must be non-empty")
    _validate_expires_format(expires)

    stmt = {
        "vulnerability": {"name": vulnerability},
        "products": [{"@id": p} for p in products],
        "status": status,
        "expires": expires,
    }
    if justification:
        stmt["justification"] = justification
    if impact_statement:
        stmt["impact_statement"] = impact_statement
    if action_statement:
        stmt["action_statement"] = action_statement
    return stmt

def _vex_document_impl(ctx):
    out = ctx.actions.declare_file(ctx.label.name + ".openvex.json")
    ctx.actions.write(out, ctx.attr.content + "\n")
    return [DefaultInfo(files = depset([out]))]

_vex_document_rule = rule(
    implementation = _vex_document_impl,
    attrs = {
        "content": attr.string(mandatory = True),
    },
    doc = "Internal: writes a pre-encoded OpenVEX JSON string to a file. " +
          "Use the vex_document macro instead.",
)

def vex_document(name, id, author, timestamp, statements, version = 1, **kwargs):
    """Emit an OpenVEX 0.2.0 JSON document.

    The document is JSON-encoded at loading phase and written to
    `<name>.openvex.json`. Because encoding happens in Starlark, the file is
    fully reproducible — it changes only when the inputs change.

    Args:
      name: Target name. Output is `<name>.openvex.json`.
      id: Stable URI identifying this document. Downstream consumers
        de-duplicate on this, so do not derive it from a label that may move.
      author: Identity string. Email or name+email is fine.
      timestamp: RFC3339 timestamp, e.g. "2026-04-30T00:00:00Z". Hardcode for
        hermiticity — auto-stamping breaks reproducibility.
      statements: List of dicts from vex_statement(). Must be non-empty.
      version: Integer document version. Bump when statements change
        materially.
      **kwargs: Forwarded to the underlying rule (visibility, tags, ...).
    """

    doc = {
        "@context": _CONTEXT,
        "@id": id,
        "author": author,
        "timestamp": timestamp,
        "version": version,
        "statements": statements,
    }
    _vex_document_rule(
        name = name,
        content = json.encode_indent(doc, indent = "  "),
        **kwargs
    )
