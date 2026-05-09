"""Cosign toolchain definition."""

CosignInfo = provider(
    doc = "Information about the cosign binary.",
    fields = {
        "cosign_binary": "The cosign executable File.",
    },
)

def _cosign_toolchain_impl(ctx):
    return [
        platform_common.ToolchainInfo(
            cosign_info = CosignInfo(
                cosign_binary = ctx.executable.cosign,
            ),
            default = ctx.attr.cosign[DefaultInfo],
        ),
    ]

cosign_toolchain = rule(
    implementation = _cosign_toolchain_impl,
    attrs = {
        "cosign": attr.label(
            mandatory = True,
            executable = True,
            cfg = "exec",
            doc = "The cosign executable.",
        ),
    },
    doc = "Defines a cosign toolchain.",
)
