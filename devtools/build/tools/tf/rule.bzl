"""`_tf_runner` — the rule emitted by `tf_root` for each `<name>.<verb>`.

Replaces the earlier `sh_binary(args = [...])` macro. The args injection
that `bazel run` uses isn't visible to direct-spawn callers (e.g.
`runnable.spawn(...)` from `aspect plan`), which left the script's `$1`
unbound. Now the per-root args are baked into a generated wrapper script
via `ctx.actions.expand_template`, so the resulting executable is
self-contained — `bazel run`, `runnable.spawn`, and bare invocation all
work the same way.

The terraform binary itself comes from a registered toolchain instead of
a hardcoded `@multitool//tools/terraform` label; see
`//devtools/build/tools/tf/toolchain`.
"""

load("//devtools/build/tools/tf/toolchain:toolchain.bzl", "TerraformInfo")

_TOOLCHAIN_TYPE = "//devtools/build/tools/tf/toolchain:toolchain_type"

def _tf_runner_impl(ctx):
    tf_info = ctx.toolchains[_TOOLCHAIN_TYPE].tf_info
    terraform = tf_info.terraform_binary

    if not ctx.files.generated:
        fail("tf_runner: `generated` must contain at least one file")
    gen_file = ctx.files.generated[0]

    # `module` entries are passed to run.sh as `<subdir>|<package>` pairs.
    # The package portion is the bazel package of the corresponding
    # filegroup — that's where its files land in the runfiles tree.
    module_entries = []
    module_files = []
    for target, subdir in ctx.attr.modules.items():
        module_entries.append("{}|{}".format(subdir, target.label.package))
        module_files.extend(target[DefaultInfo].files.to_list())

    pre_apply_files = []
    pre_apply_paths = []
    pre_apply_runfiles = ctx.runfiles()
    for pre in ctx.attr.pre_apply:
        info = pre[DefaultInfo]
        if info.files_to_run.executable == None:
            fail("tf_runner: pre_apply target {} has no executable".format(pre.label))
        exe = info.files_to_run.executable
        pre_apply_files.append(exe)
        pre_apply_paths.append(exe.short_path)
        pre_apply_runfiles = pre_apply_runfiles.merge(info.default_runfiles)

    out = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._template,
        output = out,
        substitutions = {
            "{TERRAFORM_PATH}": terraform.short_path,
            "{GEN_FILE}": gen_file.short_path,
            "{VERB}": ctx.attr.verb,
            "{ROOT_NAME}": ctx.attr.root_name,
            "{TFVARS_NL}": "\n".join([f.short_path for f in ctx.files.tfvars]),
            "{MODULES_NL}": "\n".join(module_entries),
            "{PRE_APPLY_NL}": "\n".join(pre_apply_paths),
        },
        is_executable = True,
    )

    runfiles = ctx.runfiles(
        files = (
            [terraform, ctx.file._run_sh] +
            ctx.files.generated +
            ctx.files.tfvars +
            module_files +
            pre_apply_files
        ),
    )
    runfiles = runfiles.merge(pre_apply_runfiles)

    return [DefaultInfo(executable = out, runfiles = runfiles)]

tf_runner = rule(
    implementation = _tf_runner_impl,
    executable = True,
    attrs = {
        "verb": attr.string(
            mandatory = True,
            values = ["plan", "apply", "destroy"],
        ),
        "root_name": attr.string(
            mandatory = True,
            doc = "Stable workdir key (typically <package>_<name>).",
        ),
        "generated": attr.label_list(
            allow_files = True,
            mandatory = True,
            doc = "Generated `.tf.json` files (backend + main).",
        ),
        "tfvars": attr.label_list(
            allow_files = True,
            doc = "`*.auto.tfvars.json` files copied into the workdir.",
        ),
        "modules": attr.label_keyed_string_dict(
            allow_files = True,
            doc = "filegroup label → subdir name. The filegroup's files are " +
                  "copied into <workdir>/<subdir>/.",
        ),
        "pre_apply": attr.label_list(
            cfg = "target",
            doc = "Executables run (in order) before `terraform apply`.",
        ),
        "_template": attr.label(
            default = "//devtools/build/tools/tf:runner.sh.tpl",
            allow_single_file = True,
        ),
        "_run_sh": attr.label(
            default = "//devtools/build/tools/tf:run.sh",
            allow_single_file = True,
        ),
    },
    toolchains = [_TOOLCHAIN_TYPE],
)
