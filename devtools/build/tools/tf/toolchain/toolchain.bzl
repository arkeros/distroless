"""Terraform toolchain definitions."""

TerraformInfo = provider(
    doc = "Information about the terraform binary.",
    fields = {
        "terraform_binary": "The terraform executable File.",
    },
)

def _tf_toolchain_impl(ctx):
    return [
        platform_common.ToolchainInfo(
            tf_info = TerraformInfo(
                terraform_binary = ctx.executable.terraform,
            ),
        ),
    ]

tf_toolchain = rule(
    implementation = _tf_toolchain_impl,
    attrs = {
        "terraform": attr.label(
            mandatory = True,
            executable = True,
            cfg = "exec",
            doc = "The terraform executable.",
        ),
    },
    doc = "Defines a terraform toolchain.",
)
