"""Linter aspects, one per language, for `aspect lint` and `--config=lint`.

Each is a rules_lint aspect factory bound to the tool Bazel fetches for it.
`aspect lint` (.aspect/config.axl) runs them and prints the findings; the
`lint` config in .bazelrc applies the same aspects under plain Bazel and
fails the build on a finding, which without the CLI is the only way one
reaches the terminal, as the aspects otherwise write their reports under
bazel-out and exit zero.

Buildifier visits `bzl_library` targets, so every .bzl file gazelle knows
about, and any filegroup tagged `starlark`: //:starlark and
//bazel/include:module_files hold the BUILD and MODULE files. ShellCheck
visits sh_* rules, so a script Bazel never runs is wrapped in an sh_library
for it. oxlint is this repo's own aspect, see oxlint.bzl.
"""

load("@aspect_rules_lint//lint:buildifier.bzl", "lint_buildifier_aspect")
load("@aspect_rules_lint//lint:shellcheck.bzl", "lint_shellcheck_aspect")
load("//tools/lint:oxlint.bzl", "lint_oxlint_aspect")

buildifier = lint_buildifier_aspect(
    # Check mode: the aspect's default is buildifier's own, `fix`, which
    # rewrites a misformatted file in place and, in the sandbox, fails with
    # "operation not permitted" instead of saying what is wrong.
    args = ["-mode=check"],
    binary = Label("@com_github_bazelbuild_buildtools//buildifier"),
    # buildifier's default set, not rules_lint's `all`: `all` adds opinions
    # such as sorted dict keys that the codebase orders by meaning.
    warnings = "default",
)

shellcheck = lint_shellcheck_aspect(
    binary = Label("@aspect_rules_lint//lint:shellcheck_bin"),
    config = Label("//:.shellcheckrc"),
)

oxlint = lint_oxlint_aspect(
    binary = Label("@multitool//tools/oxlint"),
    config = Label("//:.oxlintrc.json"),
)
