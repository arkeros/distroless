"""Formatters, one per language, behind `bazel run //:format`.

Every tool comes through Bazel, and none through npm. The JavaScript, JSON,
CSS and YAML formatter is oxfmt, one Rust binary with prettier's output,
pinned in //tools:tools.lock.json next to the other binaries the dev env
carries, so prettier and the node_modules it would need stay out of the
repo. It reads //:.oxfmtrc.json, walking up from each file, so it applies
from a subdirectory too. Markdown and HTML are not formatted: rules_lint
formats those with prettier only, and the HTML files here are Go templates.

`bazel run //:format` rewrites files in place, over the whole checkout or
over the paths given after `--`. `format_test` is the check, run by
`bazel test //...`: it walks the checkout with `git ls-files` rather than
listing sources, which makes it a test Bazel can neither cache nor sandbox,
and it is tagged as such. The two targets must name the same languages and
tools; a BUILD file cannot share them with `**kwargs`.
"""

load("@aspect_rules_lint//format:defs.bzl", "format_multirun", "format_test")

package(default_visibility = ["//:__subpackages__"])

OXFMT = "@multitool//tools/oxfmt"

# rules_lint drives these languages with prettier's flags, `--log-level`
# among them, which oxfmt does not have; these replace them.
OXFMT_CHECK = ["--check"]

OXFMT_FIX = ["--write"]

GOFUMPT = "@aspect_rules_lint//format:gofumpt"

SHFMT = "@aspect_rules_lint//format:shfmt"

# The buildifier go.mod pins, the same one the dev env and //tools/lint run.
BUILDIFIER = "@com_github_bazelbuild_buildtools//buildifier"

# The terraform the dev env runs, from //tools:tools.lock.json; it shadows
# the older one rules_lint's lockfile pins under the same name.
TERRAFORM = "@multitool//tools/terraform"

format_multirun(
    name = "format",
    css = OXFMT,
    css_check_args = OXFMT_CHECK,
    css_fix_args = OXFMT_FIX,
    go = GOFUMPT,
    # `javascript` covers JSON and TypeScript as well.
    javascript = OXFMT,
    javascript_check_args = OXFMT_CHECK,
    javascript_fix_args = OXFMT_FIX,
    shell = SHFMT,
    starlark = BUILDIFIER,
    terraform = TERRAFORM,
    yaml = OXFMT,
    yaml_check_args = OXFMT_CHECK,
    yaml_fix_args = OXFMT_FIX,
)

format_test(
    name = "format_test",
    size = "small",
    css = OXFMT,
    css_check_args = OXFMT_CHECK,
    fix_target = ":format",
    go = GOFUMPT,
    javascript = OXFMT,
    javascript_check_args = OXFMT_CHECK,
    no_sandbox = True,
    shell = SHFMT,
    starlark = BUILDIFIER,
    terraform = TERRAFORM,
    workspace = "//:MODULE.bazel",
    yaml = OXFMT,
    yaml_check_args = OXFMT_CHECK,
)
