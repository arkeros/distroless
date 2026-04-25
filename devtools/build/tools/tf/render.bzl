"""Build-time substitution of image-push digests into a tf_root's main.tf.json.

The macro `render_main_with_image` extracts the `<registry>/<repo>@sha256:<digest>`
URI from an `image_push` target's deploy manifest and substitutes it into a
template (typically the `main.tf.json` template tf_root produces). Replaces
the older flow of shipping a `var.image` Terraform variable with an
`image.auto.tfvars.json` — keeps the digest plumbing inside Bazel.

Callers stick `IMAGE_URI` (the sentinel below) wherever they want the URI to
land in the generated JSON.
"""

load("@jq.bzl//jq:jq.bzl", "jq")
load("@rules_img//img/private/providers:deploy_info.bzl", "DeployInfo")

# Sentinel inserted in the JSON wherever the digest URI should land. Distinct
# enough to never collide with real content; not Terraform-interpretable, so a
# plan run against an unrendered template fails loudly.
IMAGE_URI = "___BAZEL_IMAGE_URI___"

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
