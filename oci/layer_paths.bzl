"""A test that no layer of an image carries the same path twice.

`flatten` merges the tars it is given and deduplicates by the path bsdtar
lists, as a string. rpm-extract names a directory `./etc`; bsdtar, and so
every `tar` rule, names it `./etc/`. Composed together both survive, and
dockerd, which strips the slash before it compares, refuses the layer:
"duplicates of file paths not supported". Docker Desktop's containerd
store loads it without complaint, so the defect passes every local run
and fails in CI. This test asks the layer itself, comparing as dockerd
does, on any machine.
"""

# tar.bzl's toolchain type, by label: the module that names it is private
# to tar.bzl, and the label is the public contract every tar rule resolves.
TAR_TOOLCHAIN_TYPE = "@tar.bzl//tar/toolchain:type"

def _layer_unique_paths_test_impl(ctx):
    bsdtar = ctx.toolchains[TAR_TOOLCHAIN_TYPE].tarinfo.binary
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.write(
        output = script,
        is_executable = True,
        content = """#!/usr/bin/env bash
set -euo pipefail
cd "$TEST_SRCDIR/$TEST_WORKSPACE"
status=0
for layer in {layers}; do
  duplicates=$("{bsdtar}" -tf "$layer" | sed 's#/$##' | sort | uniq -d)
  if [ -n "$duplicates" ]; then
    echo "$layer carries these paths more than once:"
    echo "$duplicates" | sed 's/^/  /'
    status=1
  fi
done
if [ "{expect_duplicates}" = "True" ]; then
  [ "$status" = 1 ] || {{ echo "expected duplicate paths and found none"; exit 1; }}
  exit 0
fi
exit "$status"
""".format(
            bsdtar = bsdtar.short_path,
            expect_duplicates = ctx.attr.expect_duplicates,
            layers = " ".join(['"%s"' % f.short_path for f in ctx.files.layers]),
        ),
    )
    return [DefaultInfo(
        executable = script,
        runfiles = ctx.runfiles(
            files = ctx.files.layers + [bsdtar],
            transitive_files = ctx.toolchains[TAR_TOOLCHAIN_TYPE].default.files,
        ),
    )]

layer_unique_paths_test = rule(
    implementation = _layer_unique_paths_test_impl,
    doc = "Fails if any of `layers` lists a path more than once.",
    attrs = {
        "layers": attr.label_list(
            allow_files = True,
            doc = "Layer tars, compressed or not; whatever bsdtar reads.",
        ),
        "expect_duplicates": attr.bool(
            default = False,
            doc = "Invert: pass only if a layer does carry a duplicate. For the test's own fixture.",
        ),
    },
    test = True,
    toolchains = [TAR_TOOLCHAIN_TYPE],
)
