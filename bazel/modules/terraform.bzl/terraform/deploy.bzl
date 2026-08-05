"""Deploy-DAG membership as a type, not a convention.

A repo with more than one root needs to know three things no single root can
answer alone: which nodes are real, which may be applied by CI, and what has to
exist before what. This file is where those facts get a type.

Two rules, one provider:

- `tf_root_node` is the public target of a deployable `tf_root`. A root with
  `deploy = False` gets a plain `filegroup` instead, so **membership is the
  rule class** — there is no flag to read and no tag to trust.
- `tf_deploy_task` is a node that runs an existing executable, for a step that
  belongs somewhere in the deploy order but manages no state of its own —
  pushing an image, warming a cache, running a smoke check.
- `TfDeployInfo` is what `deploy_after` requires, which is what makes an edge
  to a non-node an analysis error rather than an edge that silently resolves
  to nothing.

`deploy_after` is a real `attr.label_list`, so Bazel does the work an
orchestrator would otherwise have to do at deploy time: a label that doesn't
parse fails at load time, a label naming nothing fails at load time, and a
label naming a target that isn't a deploy node fails at analysis time with the
missing provider named. None of those can reach a deploy.

The edge is a build-graph edge as a side effect — analyzing an LB root now
analyzes the app roots it waits for. That costs analysis time and shows up in
`deps()`, and it deliberately does *not* make their outputs inputs: a deploy
dependency still isn't a build dependency, and nothing here builds, let alone
runs, what a node waits for.

Orchestrators read this back with one loading-phase `bazel query`; nothing here
walks or runs anything. See `//docs/adr/0008-derived-terraform-deploy-set.md`
in senku for why the set is derived rather than listed.
"""

load(":refs.bzl", "TfExportsInfo")

TfDeployInfo = provider(
    doc = """Marks a target as a node in the deploy DAG.

    Required by `deploy_after`, so it is also the type that makes an edge
    checkable. Carries what an orchestrator needs *after* it has the node;
    discovery itself is a `bazel query` over rule classes, which the loading
    phase can answer without analysing anything.
    """,
    fields = {
        "kind": "`root` (drive it through its `.apply` runnable) or `task` (run it).",
        "bootstrap": "True if applying this provisions the credentials CI itself runs as.",
        "after": "`Label`s of nodes that must run to completion first.",
    },
)

# Requiring the provider is the whole point: it is what turns
# `deploy_after = [\":typo\"]` into an error instead of a dropped edge.
_DEPLOY_AFTER_ATTR = attr.label_list(
    providers = [TfDeployInfo],
    doc = "Nodes that must run to completion before this one.",
)

def _after_labels(ctx):
    return [t.label for t in ctx.attr.deploy_after]

def _export_files(ctx):
    """`{export name: File}` from the `export_files` label→name mapping."""
    files = {}
    exported_by = {}
    for target, name in ctx.attr.export_files.items():
        candidates = target[DefaultInfo].files.to_list()
        if len(candidates) != 1:
            fail("export_files: {} must produce exactly one file, got {}".format(
                target.label,
                len(candidates),
            ))

        # The attribute keys the dict by label, so labels are unique and names
        # are not. Without this, the second file silently wins and a `ref` for
        # that name resolves to the wrong value with nothing to notice it.
        if name in exported_by:
            fail("export_files: {} and {} both export \"{}\"".format(
                exported_by[name],
                target.label,
                name,
            ))
        files[name] = candidates[0]
        exported_by[name] = target.label
    return files

def _tf_root_node_impl(ctx):
    return [
        DefaultInfo(
            files = depset(ctx.files.srcs),
            runfiles = ctx.runfiles(files = ctx.files.srcs),
        ),
        TfDeployInfo(
            kind = "root",
            bootstrap = ctx.attr.bootstrap,
            after = _after_labels(ctx),
        ),
        TfExportsInfo(exports = ctx.attr.exports, files = {}),
    ]

tf_root_node = rule(
    implementation = _tf_root_node_impl,
    doc = """The public target of a deployable `tf_root`.

    Stands in for the `filegroup` a non-deployable root gets, forwarding the
    same generated files. Emitted by `tf_root`; not meant to be instantiated
    directly.
    """,
    attrs = {
        "srcs": attr.label_list(
            allow_files = True,
            doc = "The root's generated files, forwarded as-is.",
        ),
        "bootstrap": attr.bool(
            default = False,
            doc = "Applying this provisions the credentials CI runs as, so CI must not.",
        ),
        "deploy_after": _DEPLOY_AFTER_ATTR,
        "exports": attr.string_dict(
            doc = "Build-time constants other roots may `ref`. See `refs.bzl`.",
        ),
    },
)

def _tf_deploy_task_impl(ctx):
    # Symlinked rather than aliased: an `alias` forwards the providers of its
    # actual, which is exactly what we cannot do here — the node has to *add*
    # `TfDeployInfo` to a target that knows nothing about deploys.
    out = ctx.actions.declare_file(ctx.label.name + ".run")
    ctx.actions.symlink(
        output = out,
        target_file = ctx.executable.run,
        is_executable = True,
    )
    return [
        DefaultInfo(
            executable = out,
            runfiles = ctx.runfiles(files = [ctx.executable.run])
                .merge(ctx.attr.run[DefaultInfo].default_runfiles),
        ),
        TfDeployInfo(
            kind = "task",
            bootstrap = False,
            after = _after_labels(ctx),
        ),
        TfExportsInfo(exports = {}, files = _export_files(ctx)),
    ]

tf_deploy_task = rule(
    implementation = _tf_deploy_task_impl,
    doc = "A deploy node that runs an existing executable. See `deploy_task`.",
    executable = True,
    attrs = {
        "run": attr.label(
            executable = True,
            cfg = "target",
            mandatory = True,
            doc = "The executable this node runs.",
        ),
        "deploy_after": _DEPLOY_AFTER_ATTR,
        "export_files": attr.label_keyed_string_dict(
            allow_files = True,
            doc = """Files whose contents are exported values, as {file: export name}.

            For a value the build computes rather than knows: an image digest
            lands in a file long before Terraform runs, which is what a `ref`
            needs, but it is not a literal anyone could write in a BUILD file.
            Keyed by label because a Starlark dict cannot be keyed by one.
            """,
        ),
    },
)

def deploy_task(name, run, after = None, export_files = None, visibility = None):
    """A deploy-DAG node that runs an existing target.

    The counterpart to `tf_root`: a node that is a plain runnable rather than
    a Terraform root. Use it for a step that has to happen in a particular
    place in the deploy order but manages no state of its own.

    Prefer this over a `tf_root` with `docs = []`. That works, but every root
    carries a backend, so a "root" managing nothing still creates a state
    object, takes an `init`/`apply` round trip on every deploy, and shows up
    as "No changes." in every plan.

    `manual` keeps it out of wildcard builds — being in the deploy DAG is what
    makes it run. The target it wraps stays independently runnable.

    Args:
        name: Target name; this is the node's label.
        run: Label of the executable to run.
        after: Labels of nodes that must run first.
        export_files: `{file label: export name}` for values this node
            publishes as file contents, for another root to `ref`.
        visibility: Standard.
    """
    tf_deploy_task(
        name = name,
        run = run,
        deploy_after = after or [],
        export_files = export_files or {},
        tags = ["manual"],
        visibility = visibility,
    )
