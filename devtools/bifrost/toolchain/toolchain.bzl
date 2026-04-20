"""Bifrost toolchain definitions."""

BifrostInfo = provider(
    doc = "Information about the bifrost binary",
    fields = {
        "bifrost_binary": "The bifrost executable File",
    },
)

def _bifrost_toolchain_impl(ctx):
    return [
        platform_common.ToolchainInfo(
            bifrost_info = BifrostInfo(
                bifrost_binary = ctx.executable.bifrost,
            ),
        ),
    ]

bifrost_toolchain = rule(
    implementation = _bifrost_toolchain_impl,
    attrs = {
        "bifrost": attr.label(
            mandatory = True,
            executable = True,
            cfg = "exec",
            doc = "The bifrost executable",
        ),
    },
    doc = "Defines a bifrost toolchain.",
)
