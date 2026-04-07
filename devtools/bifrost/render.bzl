"""Bifrost render rule that supports image resolution from image_push targets."""

load("@rules_img//img/private/providers:deploy_info.bzl", "DeployInfo")

def _bifrost_render_impl(ctx):
    spec_file = ctx.file.spec
    env_file = ctx.file.environment
    out = ctx.outputs.out
    bifrost = ctx.executable._bifrost
    jq = ctx.toolchains["@jq.bzl//jq/toolchain:type"].jqinfo.bin

    inputs = [spec_file, env_file]
    render_spec = spec_file

    # If image_push is set, patch the service JSON with the resolved image ref
    if ctx.attr.image_push:
        deploy_manifest = ctx.attr.image_push[DeployInfo].deploy_manifest
        patched_spec = ctx.actions.declare_file(ctx.label.name + ".resolved.json")

        ctx.actions.run_shell(
            tools = [jq],
            inputs = [spec_file, deploy_manifest],
            outputs = [patched_spec],
            command = """\
set -euo pipefail
IMAGE=$({jq} -re '.operations[0] // error("no operations in deploy manifest") | if (.registry == null or .repository == null or .root.digest == null) then error("missing registry, repository, or digest") else "\\(.registry)/\\(.repository)@\\(.root.digest)" end' {deploy})
{jq} --arg img "$IMAGE" '.spec.image = $img' {spec} > {out}
""".format(
                jq = jq.path,
                deploy = deploy_manifest.path,
                spec = spec_file.path,
                out = patched_spec.path,
            ),
            mnemonic = "BifrostResolveImage",
        )
        render_spec = patched_spec
        inputs = [patched_spec, env_file]

    # Run bifrost render
    if ctx.attr.header:
        ctx.actions.run_shell(
            tools = [bifrost],
            inputs = inputs,
            outputs = [out],
            command = "cat <<'BIFROST_HEADER_EOF' > {out}\n{header}\nBIFROST_HEADER_EOF\n{bifrost} {target} render -e {env} -f {spec} >> {out}".format(
                bifrost = bifrost.path,
                target = ctx.attr.target,
                env = env_file.path,
                spec = render_spec.path,
                out = out.path,
                header = ctx.attr.header,
            ),
            mnemonic = "BifrostRender",
        )
    else:
        ctx.actions.run_shell(
            tools = [bifrost],
            inputs = inputs,
            outputs = [out],
            command = "{bifrost} {target} render -e {env} -f {spec} > {out}".format(
                bifrost = bifrost.path,
                target = ctx.attr.target,
                env = env_file.path,
                spec = render_spec.path,
                out = out.path,
            ),
            mnemonic = "BifrostRender",
        )

    return [DefaultInfo(files = depset([out]))]

bifrost_render = rule(
    implementation = _bifrost_render_impl,
    attrs = {
        "spec": attr.label(
            mandatory = True,
            allow_single_file = [".json"],
            doc = "Bifrost workload spec JSON file.",
        ),
        "environment": attr.label(
            mandatory = True,
            allow_single_file = [".json", ".yaml", ".yml"],
            doc = "Bifrost environment file (YAML or JSON).",
        ),
        "target": attr.string(
            mandatory = True,
            values = ["cloudrun", "k8s", "terraform"],
            doc = "Render target: cloudrun, k8s, or terraform.",
        ),
        "image_push": attr.label(
            doc = "Optional image_push target. When set, the deploy manifest is read to resolve a digest-pinned image reference.",
        ),
        "header": attr.string(
            doc = "Optional header to prepend to the output file.",
        ),
        "out": attr.output(
            mandatory = True,
            doc = "Output file name.",
        ),
        "_bifrost": attr.label(
            default = "//devtools/bifrost/cli",
            executable = True,
            cfg = "exec",
        ),
    },
    toolchains = ["@jq.bzl//jq/toolchain:type"],
)
