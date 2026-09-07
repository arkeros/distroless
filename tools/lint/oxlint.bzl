"""An oxlint aspect in the shape of rules_lint's own linters.

rules_lint has no oxlint: its JavaScript linter is ESLint, installed from
npm, which this repo keeps out (see //tools/format). oxlint is one binary
from the oxc project, pinned in //tools:tools.lock.json, with SARIF output
built in.

The aspect keeps rules_lint's conventions so its reports look like the
others': the action mnemonic starts with `AspectRulesLint`, the
human-readable report and the SARIF report go to the `rules_lint_human` and
`rules_lint_machine` output groups, and rules_lint's `fail_on_violation`
flag decides whether a finding fails the action or is recorded as an exit
code beside the report. What it does not do is load rules_lint's private
helpers: the bzl_library that owns them is not visible from here, and
gazelle would add it as a dep, so the few lines they would save are
written out below.

It visits rules_js binaries, libraries and tests and esbuild bundles, and
lints their `srcs` and `entry_point`: the entry point of a js_test or a
bundle is a source file too, and not in `srcs`. As with rules_lint's
aspects, a target tagged `no-lint` is skipped, and generated sources are
unless the target is tagged `lint-genfiles`.
"""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

_MNEMONIC = "AspectRulesLintOxlint"

_EXTENSIONS = ["js", "mjs", "cjs", "jsx", "ts", "mts", "cts", "tsx"]

def oxlint_action(ctx, executable, srcs, config, stdout, exit_code = None, format = "default"):
    """Run oxlint as an action under Bazel.

    Args:
        ctx: Bazel Rule or Aspect evaluation context
        executable: the oxlint program
        srcs: JavaScript and TypeScript files to lint
        config: the .oxlintrc.json file
        stdout: output file for what oxlint prints
        exit_code: output file for oxlint's exit code. If None, the action
            fails when oxlint exits non-zero, that is on any error-level
            finding, and what oxlint printed is the failure's output.
        format: oxlint's `--format`; `sarif` for the machine-readable report
    """
    args = ctx.actions.args()
    args.add("--config", config)
    args.add("--format", format)
    args.add_all(srcs)
    outputs = [stdout]
    if exit_code:
        command = '{oxlint} "$@" >{stdout}; echo $? >{exit_code}'.format(
            oxlint = executable.path,
            stdout = stdout.path,
            exit_code = exit_code.path,
        )
        outputs.append(exit_code)
    else:
        command = '{oxlint} "$@" && touch {stdout}'.format(
            oxlint = executable.path,
            stdout = stdout.path,
        )
    ctx.actions.run_shell(
        inputs = srcs + [config],
        outputs = outputs,
        command = command,
        arguments = [args],
        mnemonic = _MNEMONIC,
        progress_message = "Linting %{label} with oxlint",
        tools = [executable],
    )

def _files_to_lint(rule):
    files = getattr(rule.files, "srcs", []) + getattr(rule.files, "entry_point", [])
    if "lint-genfiles" not in rule.attr.tags:
        files = [f for f in files if f.is_source and f.owner.workspace_name == ""]

    # An entry point may be listed in srcs as well; lint it once.
    return [f for f in {f: None for f in files} if f.extension in _EXTENSIONS]

def _oxlint_aspect_impl(target, ctx):
    if ctx.rule.kind not in ctx.attr._rule_kinds or "no-lint" in ctx.rule.attr.tags:
        return []

    name = "{}.{}".format(target.label.name, _MNEMONIC)
    human = ctx.actions.declare_file(name + ".out")
    machine = ctx.actions.declare_file(name + ".report")
    human_exit_code = None
    machine_exit_code = None
    if not ctx.attr._fail_on_violation[BuildSettingInfo].value:
        human_exit_code = ctx.actions.declare_file(name + ".out.exit_code")
        machine_exit_code = ctx.actions.declare_file(name + ".report.exit_code")
    exit_codes = [f for f in [human_exit_code, machine_exit_code] if f]

    srcs = _files_to_lint(ctx.rule)
    if srcs:
        oxlint_action(ctx, ctx.executable._oxlint, srcs, ctx.file._config, human, human_exit_code)
        oxlint_action(ctx, ctx.executable._oxlint, srcs, ctx.file._config, machine, machine_exit_code, format = "sarif")
    else:
        # Given no paths, oxlint would lint the working directory instead.
        ctx.actions.run_shell(
            outputs = [human, machine] + exit_codes,
            command = " && ".join(
                ["touch {} {}".format(human.path, machine.path)] +
                ["echo 0 > {}".format(f.path) for f in exit_codes],
            ),
        )

    return [OutputGroupInfo(
        rules_lint_human = depset([f for f in [human, human_exit_code] if f]),
        rules_lint_machine = depset([f for f in [machine, machine_exit_code] if f]),
        # Validation outputs are built whenever the target is, so the
        # linter runs without anyone asking for its report.
        _validation = depset([human]),
    )]

def lint_oxlint_aspect(binary, config, rule_kinds = ["js_binary", "js_library", "js_test", "esbuild_bundle"]):
    """A factory function to create an oxlint linter aspect.

    Args:
        binary: an oxlint executable
        config: the .oxlintrc.json file
        rule_kinds: which rule kinds the aspect lints

    Returns:
        an aspect, to name in `--aspects` or pass to rules_lint's `lint_test`
    """
    return aspect(
        implementation = _oxlint_aspect_impl,
        attrs = {
            "_oxlint": attr.label(
                default = binary,
                executable = True,
                cfg = "exec",
            ),
            "_config": attr.label(
                default = config,
                allow_single_file = True,
            ),
            "_rule_kinds": attr.string_list(
                default = rule_kinds,
            ),
            "_fail_on_violation": attr.label(
                default = Label("@aspect_rules_lint//lint:fail_on_violation"),
            ),
        },
    )
