"""Cosign toolchain definition."""

CosignInfo = provider(
    doc = "Information about the cosign binary.",
    fields = {
        "cosign_binary": "The cosign executable File.",
    },
)

def _cosign_toolchain_impl(ctx):
    default_info = ctx.attr.cosign[DefaultInfo]
    return [
        platform_common.ToolchainInfo(
            cosign_info = CosignInfo(
                cosign_binary = ctx.executable.cosign,
            ),
            default = default_info,
        ),
        # Re-publish the binary's files + runfiles so consumers can
        # `bazel cquery <toolchain> --output=files` without reaching into
        # the underlying binary label. We can't forward the source rule's
        # `DefaultInfo` directly because Bazel forbids re-exporting another
        # rule's `executable` from an executable rule.
        DefaultInfo(
            files = depset([ctx.executable.cosign]),
            runfiles = default_info.default_runfiles,
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
