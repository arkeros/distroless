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
load("@rules_img//img/private/providers:deploy_info.bzl", "DeployInfo")
load("@terraform.bzl", _tf_root = "tf_root")

# Sentinel inserted in the JSON wherever the digest URI should land. Distinct
# enough to never collide with real content; not Terraform-interpretable, so a
# plan run against an unrendered template fails loudly.
IMAGE_URI = "___BAZEL_IMAGE_URI___"

# Deploy tiers. `aspect apply` walks the deploy set tier by tier, ascending.
#
# The registry sits at tier 0 because everything else is defined relative to
# it. Any root that pushes an image needs the Artifact Registry repository to
# already exist — `tf_root_with_image` prepends an `image_push` to `pre_apply`,
# and that push happens before terraform runs — so every such root is tier 1
# by construction, with no per-call-site declaration. That is the whole point:
# a new app inherits its position from the macro it already uses.
REGISTRY_DEPLOY_TIER = 0

IMAGE_ROOT_DEPLOY_TIER = 1

# The load balancer goes last. Its serverless NEGs name Cloud Run services by
# string, and a NEG whose service does not exist resolves to a bare 404 rather
# than an error — so every backend must be applied first. Tier 2 clears both
# tier 0 and every image-pushing root at tier 1.
LB_DEPLOY_TIER = IMAGE_ROOT_DEPLOY_TIER + 1

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

    # Pushing an image means the registry has to exist already, so an
    # image-pushing root is never tier 0. Callers may still override — a
    # future root might need to land later still — but they never have to
    # think about it to be correct.
    kwargs.setdefault("deploy_tier", IMAGE_ROOT_DEPLOY_TIER)

    _tf_root(
        name = name,
        pre_apply = pre,
        main_postprocess = _postprocess,
        **kwargs
    )
