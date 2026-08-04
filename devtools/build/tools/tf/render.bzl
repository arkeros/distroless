"""Build-time substitution of image-push digests into a tf_root's main.tf.json.

`render_main_with_image` extracts the `<registry>/<repo>@sha256:<digest>` URI
from an `image_push` target's deploy manifest and substitutes it into a
template (the `main.tf.json` template tf_root produces). Replaces the older
flow of shipping a `var.image` Terraform variable with an
`image.auto.tfvars.json` — keeps the digest plumbing inside Bazel.

`tf_root_with_image` wraps the module's plain `tf_root` with two extras:

  1. `main_postprocess` is bound to `render_main_with_image` so the IMAGE_URI
     sentinel in `docs` becomes a digest-pinned reference by the time
     `main.tf.json` lands in bazel-bin.
  2. `image_push` is auto-prepended to `pre_apply` so `aspect apply` pushes
     the image before `terraform apply` reads from the registry.

The module's `tf_root` itself stays generic — only senku needs this glue
because only senku consumes rules_img's `DeployInfo`.
"""

load("@jq.bzl//jq:jq.bzl", "jq")
load("@rules_img//img:push.bzl", "image_push")
load("@rules_img//img/private/providers:deploy_info.bzl", "DeployInfo")
load("@terraform.bzl", _deploy_tags = "deploy_tags", _tf_root = "tf_root")

# Sentinel inserted in the JSON wherever the digest URI should land. Distinct
# enough to never collide with real content; not Terraform-interpretable, so a
# plan run against an unrendered template fails loudly.
IMAGE_URI = "___BAZEL_IMAGE_URI___"

# Every deployed image is tagged the same way: a moving `latest` plus two
# immutable stamps. Callers that need something else (the registry's debug
# variant) pass their own.
DEFAULT_TAG_LIST = [
    "latest",
    "{{.STABLE_MONOREPO_IMAGE_TAG_VERSION}}",
    "{{.STABLE_MONOREPO_SHORT_VERSION}}",
]

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
    tags = ["manual"]
    if deploy:
        tags += _deploy_tags(
            after = [registry.root],
            context = "registry_push({})".format(name),
        )

    image_push(
        name = name,
        image = image,
        registry = registry.host,
        repository = registry.repository_prefix + "/" + repository,
        # Immutable version tags are only meaningful when the workspace status
        # is stamped in, and every caller wants them.
        stamp = "force",
        tag_list = tag_list or DEFAULT_TAG_LIST,
        tags = tags,
        **kwargs
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

def render_main_with_image(name, template, image_push, out):
    """Substitute `IMAGE_URI` in `template` with `image_push`'s digest URI.

    Three actions chain: deploy_manifest → jq (extract URI as raw text) →
    genrule (sed substitute into the template).

    Args:
        name: Target name (the genrule emitting `out`).
        template: Label of the template file (the unsubstituted main.tf.json).
        image_push: Label of an `image_push` target (provides DeployInfo).
        out: Path of the rendered output file (relative to package).
    """
    manifest_target = "_{}_manifest".format(name)
    uri_target = "_{}_uri".format(name)

    _deploy_manifest(
        name = manifest_target,
        image_push = image_push,
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

    native.genrule(
        name = name,
        srcs = [template, ":" + uri_target],
        outs = [out],
        cmd = """\
set -euo pipefail
URI=$$(cat $(execpath :{uri}))
[ -n "$$URI" ] || (echo "render_main_with_image: empty URI from {uri}" >&2; exit 1)
sed "s|{placeholder}|$$URI|g" $(execpath {template}) > $@
""".format(
            uri = uri_target,
            template = template,
            placeholder = IMAGE_URI,
        ),
    )

def tf_root_with_image(name, image_push, pre_apply = None, **kwargs):
    """`tf_root` + IMAGE_URI substitution + auto-push pre_apply hook.

    Wraps the module's plain `tf_root` for the (senku-specific) case
    where `docs` contains `IMAGE_URI` sentinels that should resolve to
    a digest-pinned `<registry>/<repo>@sha256:...` reference at Bazel
    build time, and where the image must be pushed to the registry
    before `terraform apply` reads it back.

    Args:
        name: Same as `tf_root`.
        image_push: Label of an `image_push` target. Its `DeployInfo.deploy_manifest`
            drives both the URI substitution and the pre-apply push.
        pre_apply: Optional list of additional executables run before
            `terraform apply`. `image_push` is auto-prepended so the
            image lands in the registry before terraform reads it; if
            you pass `image_push` here yourself, no double-push.
        **kwargs: Forwarded to `tf_root` (docs, providers, backend_bucket, ...).
    """
    pre = list(pre_apply or [])
    if image_push not in pre:
        pre = [image_push] + pre

    # Nested `def` so the closure captures `image_push` from the
    # enclosing scope. `tf_root`'s `main_postprocess` contract is
    # `(name, template, out) -> None` — three args — but
    # `render_main_with_image` needs a fourth (`image_push`), so we
    # have to pre-bind it. Starlark has no `functools.partial` and no
    # lambdas, so a nested `def` is the only way to produce a callback
    # with the right arity. Each `tf_root_with_image` call creates a
    # fresh closure with its own `image_push`.
    def _postprocess(name, template, out):
        render_main_with_image(
            name = name,
            template = template,
            image_push = image_push,
            out = out,
        )

    # The push is a deploy node in its own right (see `registry_push`), and this
    # root consumes what it produces: `main.tf.json` already carries the
    # digest, but the image only exists at that digest once the push has run.
    # So the edge is root-after-push, and the push carries the edge to the
    # registry — nobody declares "this app needs GAR", because the app's
    # Terraform doesn't; its push does.
    kwargs["deploy_after"] = [image_push] + list(kwargs.get("deploy_after") or [])

    _tf_root(
        name = name,
        pre_apply = pre,
        main_postprocess = _postprocess,
        **kwargs
    )
