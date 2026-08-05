"""Cross-root references: a root publishes `exports`, another names one with `ref`.

`ref("//infra/cloud/gcp/gar:terraform", "registry_host")` is a sentinel string.
It survives ordinary string building — concatenation, `%`, `.format()` — because
it *is* a string, and it is replaced with the exported value by an analysis-time
action before Terraform ever sees the JSON.

The point is that the reference and the dependency are the same token. A root
that names another root's value cannot fail to declare the deploy edge, because
`tf_root` derives the edge from the sentinels it finds in the document. There is
no second field to keep in agreement with the first.

This generalises what senku's image-digest plumbing used to do on its own:
emit a sentinel, substitute it before Terraform runs, and take the deploy edge
from the same label. That was one hardcoded value with a bespoke genrule; this
is the same idea with a name.

**Exports are build-time values, not Terraform outputs.** Usually literals; a
node may also export the *contents of a file* the build produces, for something
it computes rather than knows — an image digest, say. Either way the value is
settled before Terraform runs, which is what makes it substitutable. A value
Terraform itself computes at apply time (an allocated IP, a generated URL) is
not: publish those with `output()` and read them with `remote_state()`. The two
mechanisms are deliberately separate and named separately.

A `ref` always implies a deploy edge, because naming another root's resource
means that resource must exist. For a value the provider never resolves — a
bucket name interpolated into a log filter, say — that is the wrong claim: keep
`load()`ing the constant, and no edge is created.
"""

TfExportsInfo = provider(
    doc = """Build-time values a node publishes for other roots to `ref`.

    Distinct from Terraform's `output`: these are produced by the build,
    never read from state.

    Two kinds, because not every build-time value is a literal. A service
    name is known while the BUILD file is being evaluated; an image digest
    is the content of a file some action produces. Both are settled before
    Terraform runs, which is the property that matters, so `ref` does not
    distinguish them — only the resolver does.
    """,
    fields = {
        "exports": "dict of export name -> literal string, substituted at analysis time",
        "files": "dict of export name -> File whose contents are the value, substituted at action time",
    },
)

_MARKER = "___BAZEL_REF_"

_END = "___"

_SEP = "#"

# Bound on the document walk. A root's JSON is a few thousand nodes; this is
# high enough to never bind in practice and low enough to fail rather than hang
# if a document is somehow cyclic.
_MAX_NODES = 1000000

def ref(root, name):
    """Reference another root's export by label.

    Args:
        root: Absolute label of the producing root (`//pkg:terraform`).
            Absolute because this is typically written in a `defs.bzl` that
            another package loads, where `native.package_name()` would name
            the loader rather than the producer.
        name: Key in that root's `exports`.

    Returns:
        A sentinel string, substituted with the exported value at analysis
        time. Safe to concatenate or interpolate into a larger string.
    """
    if not root.startswith("//"):
        fail("ref: `root` must be an absolute label (`//pkg:target`), got {}".format(root))
    if _SEP in root or _SEP in name:
        fail("ref: `{}` / `{}` may not contain {}".format(root, name, _SEP))
    if _MARKER in root or _MARKER in name:
        fail("ref: `{}` / `{}` may not contain the reference marker {}".format(root, name, _MARKER))

    # Checked after the marker, which starts with `_END`, so a name carrying a
    # whole marker gets the more specific message. `collect_refs` reads up to
    # the first `_END`, so an embedded one would silently truncate the key.
    if _END in root or _END in name:
        fail("ref: `{}{}{}` may not contain {}".format(root, _SEP, name, _END))
    return _MARKER + root + _SEP + name + _END

def ref_key_label(key):
    """The label half of a `<label>#<name>` reference key."""
    return key.split(_SEP)[0]

def ref_sentinel(key):
    """The sentinel text a `<label>#<name>` reference key appears as."""
    return _MARKER + key + _END

def collect_refs(doc):
    """Every `<label>#<name>` referenced anywhere in a JSON-able document.

    Iterative rather than recursive: Starlark forbids recursion, and a
    Terraform document nests arbitrarily deep.
    """
    found = {}
    stack = [doc]
    for _ in range(_MAX_NODES):
        if not stack:
            break
        value = stack.pop()
        kind = type(value)
        if kind == "string":
            for part in value.split(_MARKER)[1:]:
                if _END not in part:
                    fail("tf_root: malformed reference sentinel in {}".format(value))
                found[part.split(_END)[0]] = None
        elif kind == "dict":
            for k, v in value.items():
                stack.append(k)
                stack.append(v)
        elif kind == "list" or kind == "tuple":
            for item in value:
                stack.append(item)
    if stack:
        fail("tf_root: document too large or cyclic to scan for references")
    return sorted(found)

def check_exports(name, exports):
    """Reject exports that cannot be substituted into another root's JSON."""
    for key, value in exports.items():
        if type(value) != "string":
            fail("tf_root({}): export {} must be a string, got {}".format(name, key, type(value)))
        if "${" in value:
            fail(
                ("tf_root({}): export {} is a Terraform interpolation. Exports are " +
                 "resolved by Bazel at analysis time, so they must be literals. For a " +
                 "value Terraform computes at apply time, publish it with `output()` " +
                 "and read it with `remote_state()`.").format(name, key),
            )
        if "\"" in value or "\\" in value or "\n" in value:
            fail(
                ("tf_root({}): export {} contains a quote, backslash or newline. " +
                 "Substitution happens on the encoded JSON, so the value must need " +
                 "no escaping.").format(name, key),
            )

def _tf_resolve_refs_impl(ctx):
    if len(ctx.attr.refs) != len(ctx.attr.ref_labels):
        fail("tf_resolve_refs: `refs` and `ref_labels` must be parallel")

    # Keyed by the label text the caller wrote rather than by `dep.label`,
    # whose string form is canonicalised (`@@//pkg:target`) and would not
    # match what `ref()` embedded in the sentinel.
    by_label = {
        ctx.attr.ref_labels[i]: dep[TfExportsInfo]
        for i, dep in enumerate(ctx.attr.refs)
    }

    literals = {}
    from_files = {}
    for key in ctx.attr.ref_keys:
        label = ref_key_label(key)
        name = key[len(label) + len(_SEP):]
        info = by_label[label]
        if name in info.exports:
            literals[ref_sentinel(key)] = info.exports[name]
        elif name in info.files:
            from_files[ref_sentinel(key)] = info.files[name]
        else:
            fail("{}: ref(\"{}\", \"{}\") — that root exports {}".format(
                ctx.label,
                label,
                name,
                sorted(list(info.exports) + list(info.files)) or "nothing",
            ))

    # Literals are settled here, so `expand_template` can place them without
    # anything having to escape them.
    expanded = ctx.outputs.out if not from_files else ctx.actions.declare_file(
        ctx.label.name + ".literals.json",
    )
    ctx.actions.expand_template(
        template = ctx.file.src,
        output = expanded,
        substitutions = literals,
    )

    # File-backed values are only readable while the action runs, so they need
    # a second pass. `sed` with `|` delimiters, matching what these values look
    # like: registry URIs and digests, no shell metacharacters.
    if from_files:
        out = ctx.outputs.out.path
        script = ["set -euo pipefail", "cp {} {}".format(expanded.path, out)]
        inputs = [expanded]
        for sentinel, f in sorted(from_files.items()):
            inputs.append(f)
            script.append("VALUE=$(cat {})".format(f.path))
            script.append(
                '[ -n "$VALUE" ] || {{ echo "tf_resolve_refs: {} is empty" >&2; exit 1; }}'.format(f.path),
            )
            script.append('sed -i.bak "s|{}|$VALUE|g" {}'.format(sentinel, out))
        script.append("rm -f {}.bak".format(out))
        ctx.actions.run_shell(
            inputs = inputs,
            outputs = [ctx.outputs.out],
            command = "\n".join(script),
            mnemonic = "TfResolveRefs",
            progress_message = "Resolving Terraform references in %s" % ctx.label,
        )

    return [DefaultInfo(files = depset([ctx.outputs.out]))]

tf_resolve_refs = rule(
    implementation = _tf_resolve_refs_impl,
    doc = """Substitute reference sentinels with the values they name.

    Emitted by `tf_root` when the document contains any. Resolution is
    analysis-time, so an export name that doesn't exist fails the build with
    the available names listed, rather than leaving an unsubstituted sentinel
    for Terraform to choke on.
    """,
    attrs = {
        "src": attr.label(
            allow_single_file = True,
            mandatory = True,
            doc = "The document still containing sentinels.",
        ),
        "refs": attr.label_list(
            providers = [TfExportsInfo],
            doc = "The referenced roots, parallel to `ref_labels`.",
        ),
        "ref_labels": attr.string_list(
            doc = "Label text as written in `ref()`, parallel to `refs`.",
        ),
        "ref_keys": attr.string_list(
            doc = "`<label>#<name>` for every reference found in the document.",
        ),
        "out": attr.output(mandatory = True),
    },
)
