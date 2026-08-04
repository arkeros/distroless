"""Container images as deploy-DAG nodes, and their digests as cross-root values.

`registry_push` pushes an image to a Terraform-managed registry and makes the
push a node in the deploy graph. The digest-pinned
`<registry>/<repo>@sha256:...` URI is published as an *export* of that node, so
a root names it with `image_uri()` — an ordinary `ref` — and gets both the
value and its deploy edge from the same token. Replaces the older flow of
shipping a `var.image` Terraform variable with an `image.auto.tfvars.json`,
and the bespoke `IMAGE_URI` sentinel that replaced that.

A root that deploys an image is then an ordinary `tf_root`: it names the digest
with `image_uri()` and lists the push in `pre_apply`, so a bare
`bazel run //x:terraform.apply` pushes before Terraform reads the registry back.
No wrapper macro left in between.

The module's `tf_root` stays generic — only senku needs this glue, because
only senku consumes rules_img's `DeployInfo`.
"""

load("@jq.bzl//jq:jq.bzl", "jq")
load("@rules_img//img:push.bzl", "image_push")
load("@rules_img//img/private/providers:deploy_info.bzl", "DeployInfo")
load("@terraform.bzl", "ref", _deploy_task = "deploy_task")

# Name the push node publishes its digest-pinned URI under. Roots reach it
# with `image_uri(":image_push_gar")` rather than naming the export directly.
IMAGE_URI_EXPORT = "image_uri"

# Every deployed image is tagged the same way: a moving `latest` plus two
# immutable stamps. Callers that need something else (the registry's debug
# variant) pass their own.
DEFAULT_TAG_LIST = [
    "latest",
    "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}",
    "{{.STABLE_MONOREPO_SHORT_VERSION}}",
]

# `registry_push(name = "x")` emits the push as `:x` and its deploy node as
# `:x.deploy`. Named once here because `image_uri()` has to derive the same
# label to build its reference.
PUSH_NODE_SUFFIX = ".deploy"

def registry_push(name, image, registry, repository, tag_list = None, deploy = True, **kwargs):
    """Push an image to a Terraform-managed registry, as a deploy-DAG node.

    This is the target that actually depends on the registry existing: it
    writes into a repository some root provisions, and fails until that root
    has applied. Carrying the edge here rather than on the consuming
    `tf_root` attaches it to the operation that has the dependency — an app's
    Terraform is perfectly happy without a registry; only its push is not.

    Args:
        name: Target name; this is the node's label.
        image: Label of the image to push.
        registry: A registry descriptor — `host`, `repository_prefix` and the
            `root` that creates it. See `//infra/cloud/gcp/gar:defs.bzl`.
            Passed rather than hardcoded so the address and the dependency
            arrive together: a caller cannot resolve where to push without
            also learning what it has to wait for.
        repository: Repository leaf under the registry's prefix — usually the
            service name.
        tag_list: Overrides `DEFAULT_TAG_LIST`.
        deploy: False builds the push target but leaves it out of the deploy
            DAG, for images that exist only as manual tooling.
        **kwargs: Forwarded to `image_push`.
    """
    image_push(
        name = name,
        image = image,
        registry = registry.host,
        repository = registry.repository_prefix + "/" + repository,
        # Immutable version tags are only meaningful when the workspace status
        # is stamped in, and every caller wants them.
        stamp = "force",
        tag_list = tag_list or DEFAULT_TAG_LIST,
        tags = ["manual"],
        **kwargs
    )

    # The node is a sibling rather than the push itself: `image_push` is
    # rules_img's rule and cannot provide the deploy providers, and wrapping it
    # in place would hide the `DeployInfo` the digest is extracted from.
    # `.deploy` follows the `.plan`/`.apply` convention — the label names an
    # operation on the target it is suffixed to.
    if deploy:
        # The digest-pinned URI, published as an export so a root can `ref` it
        # like any other cross-root value. It is a file rather than a literal
        # because only the push knows the digest, and it knows it by building
        # the image — but it is settled long before Terraform runs, which is
        # all `ref` requires.
        manifest_target = "_{}_manifest".format(name)
        uri_target = "_{}_uri".format(name)

        _deploy_manifest(
            name = manifest_target,
            image_push = ":" + name,
            visibility = ["//visibility:private"],
        )

        jq(
            name = uri_target,
            srcs = [":" + manifest_target],
            filter = '.operations[0] | "\\(.registry)/\\(.repository)@\\(.root.digest)"',
            args = ["-r"],
            out = "_{}_uri.txt".format(name),
            visibility = ["//visibility:private"],
        )

        _deploy_task(
            name = name + PUSH_NODE_SUFFIX,
            run = ":" + name,
            after = [registry.root],
            export_files = {":" + uri_target: IMAGE_URI_EXPORT},
        )

def _deploy_manifest_impl(ctx):
    return [DefaultInfo(files = depset([ctx.attr.image_push[DeployInfo].deploy_manifest]))]

_deploy_manifest = rule(
    implementation = _deploy_manifest_impl,
    attrs = {
        "image_push": attr.label(mandatory = True, providers = [DeployInfo]),
    },
    doc = "Expose an `image_push` target's `DeployInfo.deploy_manifest` as a DefaultInfo file so `jq` can consume it via `srcs`.",
)

def image_uri(image_push):
    """A `ref` to the digest-pinned URI `registry_push` publishes.

    `service_cloudrun(image = image_uri(":image_push_gar"))` resolves to
    `<registry>/<repo>@sha256:...` in the generated JSON, and gives the root
    its deploy edge on the push at the same time — the reference is the edge.

    Args:
        image_push: Label of a `registry_push` target, absolute or relative
            to the calling package.
    """
    node = image_push + PUSH_NODE_SUFFIX
    if node.startswith(":"):
        node = "//{}{}".format(native.package_name(), node)
    return ref(node, IMAGE_URI_EXPORT)
