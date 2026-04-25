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

Path resolution goes through Bazel's bash runfiles library, so the
wrapper works whether or not the runfiles symlink tree was materialized.
That keeps the workspace default `--nobuild_runfile_links` intact when
`aspect plan` spawns this wrapper directly — no analysis-cache discard
when the next command is a bare `bazel build`.
"""

load("//devtools/build/tools/tf/toolchain:toolchain.bzl", "TerraformInfo")

_TOOLCHAIN_TYPE = "//devtools/build/tools/tf/toolchain:toolchain_type"

def _rloc(f, workspace_name):
    """`File` → key the bash runfiles library's `rlocation` accepts.

    `File.short_path` is `<package>/<basename>` for main-repo files and
    `../<canonical_repo>/<package>/<basename>` for externals. `rlocation`
    wants `<canonical_repo>/<package>/<basename>` in both cases.
    """
    sp = f.short_path
    if sp.startswith("../"):
        return sp[len("../"):]
    return workspace_name + "/" + sp

def _tf_runner_impl(ctx):
    tf_info = ctx.toolchains[_TOOLCHAIN_TYPE].tf_info
    terraform = tf_info.terraform_binary
    ws = ctx.workspace_name

    if not ctx.files.generated:
        fail("tf_runner: `generated` must contain at least one file")

    gen_paths = [_rloc(f, ws) for f in ctx.files.generated]
    tfvars_paths = [_rloc(f, ws) for f in ctx.files.tfvars]

    # Each module's files are pre-enumerated as `<subdir>|<relpath>|<rloc>`
    # triples — manifest mode has no directory subtree to `cp -R`, so
    # run.sh copies file-by-file. `relpath` preserves any nested layout
    # under the module's package (e.g. `submod/foo.tf`).
    module_entries = []
    module_files = []
    for target, subdir in ctx.attr.modules.items():
        pkg = target.label.package
        pkg_prefix = pkg + "/" if pkg else ""
        for f in target[DefaultInfo].files.to_list():
            sp = f.short_path
            if not sp.startswith(pkg_prefix):
                fail("tf_runner: module file {} not under package {}".format(sp, pkg))
            relpath = sp[len(pkg_prefix):]
            module_entries.append("{}|{}|{}".format(subdir, relpath, _rloc(f, ws)))
            module_files.append(f)

    pre_apply_files = []
    pre_apply_paths = []
    pre_apply_runfiles = ctx.runfiles()
    for pre in ctx.attr.pre_apply:
        info = pre[DefaultInfo]
        if info.files_to_run.executable == None:
            fail("tf_runner: pre_apply target {} has no executable".format(pre.label))
        exe = info.files_to_run.executable
        pre_apply_files.append(exe)
        pre_apply_paths.append(_rloc(exe, ws))
        pre_apply_runfiles = pre_apply_runfiles.merge(info.default_runfiles)

    out = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._template,
        output = out,
        substitutions = {
            "{TERRAFORM_PATH}": _rloc(terraform, ws),
            "{RUN_SH_PATH}": _rloc(ctx.file._run_sh, ws),
            "{VERB}": ctx.attr.verb,
            "{ROOT_NAME}": ctx.attr.root_name,
            "{GEN_FILES_NL}": "\n".join(gen_paths),
            "{TFVARS_NL}": "\n".join(tfvars_paths),
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
    runfiles = runfiles.merge(ctx.attr._runfiles_lib[DefaultInfo].default_runfiles)

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
                  "copied into <workdir>/<subdir>/, preserving any nested " +
                  "layout under the filegroup's package.",
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
        "_runfiles_lib": attr.label(
            default = "@bazel_tools//tools/bash/runfiles",
        ),
    },
    toolchains = [_TOOLCHAIN_TYPE],
)
