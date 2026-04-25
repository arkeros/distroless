"""Test/binary rules that wrap any shell script with the terraform toolchain.

Used by the bifrost terraform modules' `lint` (sh_test) and `validate`
(sh_binary) targets. Replaces the previous `args = ["$(rootpath
@multitool//tools/terraform)"]` pattern with a real toolchain consumer:
the rule resolves the registered terraform toolchain at analysis time,
generates a wrapper that exports `TERRAFORM_BIN`, then execs the
underlying check script. The wrapper is self-contained, so direct-spawn
callers work without the `bazel run`-only args injection.
"""

_TOOLCHAIN_TYPE = "//devtools/build/tools/tf/toolchain:toolchain_type"

def _impl(ctx):
    terraform = ctx.toolchains[_TOOLCHAIN_TYPE].tf_info.terraform_binary

    out = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._template,
        output = out,
        substitutions = {
            "{TERRAFORM_PATH}": terraform.short_path,
            "{SCRIPT_PATH}": ctx.file.script.short_path,
        },
        is_executable = True,
    )

    runfiles = ctx.runfiles(
        files = [terraform, ctx.file.script] + ctx.files.srcs,
    )
    return [DefaultInfo(executable = out, runfiles = runfiles)]

_ATTRS = {
    "script": attr.label(
        mandatory = True,
        allow_single_file = True,
        doc = "The shell script to wrap. Reads `$TERRAFORM_BIN` from env.",
    ),
    "srcs": attr.label_list(
        allow_files = True,
        doc = "Files added to the runfiles tree (e.g. *.tf to lint).",
    ),
    "_template": attr.label(
        default = "//devtools/build/tools/tf:lint_wrapper.sh.tpl",
        allow_single_file = True,
    ),
}

tf_script_test = rule(
    implementation = _impl,
    test = True,
    attrs = _ATTRS,
    toolchains = [_TOOLCHAIN_TYPE],
)

tf_script_binary = rule(
    implementation = _impl,
    executable = True,
    attrs = _ATTRS,
    toolchains = [_TOOLCHAIN_TYPE],
)
