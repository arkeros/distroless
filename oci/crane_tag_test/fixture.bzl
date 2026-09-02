"""Test-only stand-in for a rules_img image target."""

def _digest_fixture_impl(ctx):
    out = ctx.actions.declare_file(ctx.label.name + ".digest")
    ctx.actions.write(out, ctx.attr.digest)
    return [
        DefaultInfo(files = depset([out])),
        OutputGroupInfo(digest = depset([out])),
    ]

digest_fixture = rule(
    implementation = _digest_fixture_impl,
    attrs = {
        "digest": attr.string(mandatory = True),
    },
    doc = "Exposes a fixed digest through the `digest` output group, like rules_img's image rules do.",
)
