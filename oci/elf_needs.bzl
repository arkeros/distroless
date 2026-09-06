"""A test that every ELF in an image can find what it loads.

The image's layers are extracted in order into a scratch root and
//oci/tools/elfneeds resolves each ELF's interpreter and DT_NEEDED
against it, as the dynamic loader will: RUNPATH with $ORIGIN, then
/etc/ld.so.conf, then the loader's default directories. No docker, both
architectures, seconds per image. See the tool for what it cannot see:
libraries opened by name at runtime.
"""

TAR_TOOLCHAIN_TYPE = "@tar.bzl//tar/toolchain:type"

def _to_platform_impl(settings, attr):
    if attr.platform == None:
        return {"//command_line_option:platforms": settings["//command_line_option:platforms"]}
    return {"//command_line_option:platforms": str(attr.platform)}

# Build the layers for the image's platform, as image_manifest does: a
# layer from a go_binary is otherwise built for the host, and a Mach-O
# would pass this test on a Mac by not being an ELF at all.
_to_platform = transition(
    implementation = _to_platform_impl,
    inputs = ["//command_line_option:platforms"],
    outputs = ["//command_line_option:platforms"],
)

def _quote(s):
    if "'" in s:
        fail("elf_needs allow entries may not contain a single quote: %r" % s)
    return "'" + s + "'"

def _image_elf_needs_test_impl(ctx):
    bsdtar = ctx.toolchains[TAR_TOOLCHAIN_TYPE].tarinfo.binary
    allows = " ".join([
        "--allow " + _quote(glob + "=" + reason)
        for glob, reason in ctx.attr.allow.items()
    ])
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.write(
        output = script,
        is_executable = True,
        content = """#!/usr/bin/env bash
set -euo pipefail
cd "$TEST_SRCDIR/$TEST_WORKSPACE"
root="$TEST_TMPDIR/root"
rm -rf "$root" && mkdir -p "$root"
for layer in {layers}; do
  "{bsdtar}" -xf "$layer" -C "$root" -o
done
if [ "{expect_missing}" = "True" ]; then
  if "{tool}" --root "$root" {allows}; then echo "expected an ELF that cannot load and found none"; exit 1; fi
  exit 0
fi
exec "{tool}" --root "$root" {allows}
""".format(
            allows = allows,
            expect_missing = ctx.attr.expect_missing,
            bsdtar = bsdtar.short_path,
            layers = " ".join(['"%s"' % f.short_path for f in ctx.files.layers]),
            tool = ctx.executable._tool.short_path,
        ),
    )
    return [DefaultInfo(
        executable = script,
        runfiles = ctx.runfiles(
            files = ctx.files.layers + [bsdtar, ctx.executable._tool],
            transitive_files = ctx.toolchains[TAR_TOOLCHAIN_TYPE].default.files,
        ),
    )]

image_elf_needs_test = rule(
    implementation = _image_elf_needs_test_impl,
    doc = "Fails if an ELF in the composed layers cannot find its interpreter or a library it links.",
    attrs = {
        "layers": attr.label_list(
            allow_files = True,
            cfg = _to_platform,
            doc = "The image's layers, in order; whatever bsdtar extracts.",
        ),
        "platform": attr.label(
            doc = "The image's platform; the layers are built for it.",
        ),
        "allow": attr.string_dict(
            doc = "Path glob of an ELF whose unresolved needs are known -> the reason. An entry that silences nothing fails the test.",
        ),
        "expect_missing": attr.bool(
            default = False,
            doc = "Invert: pass only if some ELF cannot load. For the test's own fixture.",
        ),
        "_tool": attr.label(
            default = "//oci/tools/elfneeds",
            executable = True,
            cfg = "exec",
        ),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
    test = True,
    toolchains = [TAR_TOOLCHAIN_TYPE],
)
