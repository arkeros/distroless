"""Publishing a built webroot to a GCS bucket, as a deploy-DAG node.

The static counterpart to `registry_push` in `//devtools/build/tools/tf:render.bzl`,
with one difference that is not incidental: an image push runs *before* the
Terraform root that consumes it, because Cloud Run reads the digest back out
of the registry. A bucket push runs *after*, because Terraform is what
creates the bucket. So `bucket_push` declares its own edge with
`after = [tf_root]` rather than being listed in that root's `pre_apply`.

The bucket's contents are therefore not Terraform state. That is deliberate:
a `google_storage_bucket_object` per file would put a few hundred objects
under management, re-plan every one of them whenever a content hash moved,
and still need a rule to enumerate files Starlark cannot see. What Terraform
owns is the bucket, its IAM and its lifecycle; what the push owns is the
bytes.
"""

load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@terraform.bzl", "ref", _deploy_task = "deploy_task")

# `bucket_push(name = "x")` emits the runnable as `:x` and its deploy node as
# `:x.deploy`, matching `registry_push`'s `.deploy` convention — the suffix
# names an operation on the target it is attached to.
PUSH_NODE_SUFFIX = ".deploy"

# Name the push node publishes the bucket under. Consumers reach it with
# `published_bucket(":bucket_push")` rather than naming the export directly.
BUCKET_NAME_EXPORT = "bucket_name"

def _rloc(f, workspace_name):
    """`File` → key the bash runfiles library's `rlocation` accepts.

    `short_path` is `<package>/<basename>` for main-repo files and
    `../<canonical_repo>/...` for externals; `rlocation` wants
    `<canonical_repo>/<package>/<basename>` in both cases.
    """
    sp = f.short_path
    if sp.startswith("../"):
        return sp[len("../"):]
    return workspace_name + "/" + sp

def _bucket_publisher_impl(ctx):
    ws = ctx.workspace_name

    webroot = ctx.file.webroot
    cache_rules = ctx.file.cache_rules
    publish_bin = ctx.executable._publish

    wrapper = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._template,
        output = wrapper,
        is_executable = True,
        substitutions = {
            "{BUCKET}": ctx.attr.bucket,
            "{CACHE_RULES}": _rloc(cache_rules, ws),
            "{PUBLISH_BIN}": _rloc(publish_bin, ws),
            "{WEBROOT}": _rloc(webroot, ws),
        },
    )

    runfiles = ctx.runfiles(files = [webroot, cache_rules])
    runfiles = runfiles.merge(ctx.attr._publish[DefaultInfo].default_runfiles)
    runfiles = runfiles.merge(ctx.runfiles(files = [publish_bin]))

    # `runfiles.bash` itself, without which the wrapper cannot resolve
    # anything under `--nobuild_runfile_links`.
    runfiles = runfiles.merge(ctx.attr._runfiles_lib[DefaultInfo].default_runfiles)

    return [DefaultInfo(executable = wrapper, runfiles = runfiles)]

_bucket_publisher = rule(
    implementation = _bucket_publisher_impl,
    executable = True,
    attrs = {
        "bucket": attr.string(mandatory = True),
        "cache_rules": attr.label(mandatory = True, allow_single_file = True),
        "webroot": attr.label(mandatory = True, allow_single_file = True),
        "_publish": attr.label(
            default = "//devtools/build/tools/webroot/cmd/publish",
            executable = True,
            cfg = "exec",
        ),
        "_runfiles_lib": attr.label(
            default = "@bazel_tools//tools/bash/runfiles",
        ),
        "_template": attr.label(
            default = "//devtools/build/tools/tf:publish.sh.tpl",
            allow_single_file = True,
        ),
    },
    doc = "Wrapper that runs the webroot publisher against one bucket with its app's cache rules.",
)

def bucket_push(name, webroot, cache_rules, bucket, after, deploy = True, visibility = None, **kwargs):
    """Publish a built webroot to a GCS bucket, as a node in the deploy DAG.

    Args:
        name: Target name; this is the node's label.
        webroot: Label of the directory to publish — `react_static_layer`'s
            `:{name}_webroot`.
        cache_rules: Label of the cache policy JSON — `react_static_layer`'s
            `:{name}_cache_rules`.
        bucket: Name of the bucket to write into. A literal rather than a
            `ref()`: the name is chosen by the same Starlark that names it
            to Terraform, so there is nothing to resolve at deploy time, and
            the ordering that a `ref` would otherwise carry is stated
            directly by `after`.
        after: Labels of nodes that must run first. The Terraform root that
            creates the bucket belongs here — without it the first deploy of
            a new app races, and writes into a bucket that does not exist
            yet.
        deploy: False builds the target but leaves it out of the deploy DAG,
            for a bucket published by hand.
        visibility: Applied to both the runnable and its deploy node. The
            node needs it whenever another package resolves
            `published_bucket()` against this push — the LB root does — so
            the two are set together rather than leaving the node private
            behind a public runnable.
        **kwargs: Forwarded to the publisher target (tags, testonly).
    """
    if not after:
        fail(
            "bucket_push(%r): `after` must name the root that creates bucket %r. " % (name, bucket) +
            "A push with no ordering will run before the bucket exists.",
        )

    _bucket_publisher(
        name = name,
        webroot = webroot,
        cache_rules = cache_rules,
        bucket = bucket,
        # Publishing is a deploy action, not something a wildcard build
        # should pick up.
        tags = ["manual"],
        visibility = visibility,
        **kwargs
    )

    if deploy:
        # The bucket's name, published as an export so a consumer can `ref`
        # it. The value is a constant this macro was handed — the point is
        # not to discover it but to make naming it wait for the push. A
        # backend bucket built from `ref(":bucket_push.deploy",
        # "bucket_name")` cannot go live pointing at an empty bucket, which
        # is what a `ref` to the *root* would allow on a first deploy: the
        # bucket would exist, the site would be a 404, and nothing would say
        # so. See ADR 0008 for what that failure costs to diagnose.
        name_file = "_{}_bucket_name".format(name)
        write_file(
            name = name_file,
            out = name_file + ".txt",
            content = [bucket],
            visibility = ["//visibility:private"],
        )

        _deploy_task(
            name = name + PUSH_NODE_SUFFIX,
            run = ":" + name,
            after = after,
            export_files = {":" + name_file: BUCKET_NAME_EXPORT},
            visibility = visibility,
        )

def published_bucket(bucket_push):
    """A `ref` to the bucket name `bucket_push` publishes.

    `LB_BACKEND = {"bucket_name": published_bucket("//apps/x:bucket_push")}`
    resolves to the bucket's name in the generated JSON and gives the LB its
    deploy edge on the push at the same time — the reference is the edge, so
    routing cannot be created before there is anything to route to.

    Args:
        bucket_push: Label of a `bucket_push` target, absolute or relative
            to the calling package.
    """
    node = bucket_push + PUSH_NODE_SUFFIX
    if node.startswith(":"):
        node = "//{}{}".format(native.package_name(), node)
    return ref(node, BUCKET_NAME_EXPORT)
