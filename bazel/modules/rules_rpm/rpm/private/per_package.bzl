"""`rpm_package`: one (package, arch) pair as a Bazel target.

The rule runs the `rpm-extract` Go binary as a build action: given a .rpm
file it produces `<name>.content.tar` plus `<name>.header.blob`. Two
providers come out:

- DefaultInfo: the content tar. Drop the target straight into
  `flatten(tars = [...])` like any other layer source.
- RpmHeaderInfo: the raw RPM general-header blob plus identity metadata.
  Picked up by the `gather_rpm_headers` aspect (see private/gather.bzl)
  so consumers don't have to enumerate header files manually.

Repo rules can't invoke `rpm-extract` directly because they run before the
go_binary is compiled. Doing extraction as an analysis-phase action keeps
the toolchain hermetic and lets Bazel cache per-rpm extraction normally.
"""

load(":providers.bzl", "RpmHeaderInfo")

def _rpm_package_impl(ctx):
    content_tar = ctx.actions.declare_file(ctx.label.name + ".content.tar")
    header_blob = ctx.actions.declare_file(ctx.label.name + ".header.blob")

    args = ctx.actions.args()
    args.add("--rpm", ctx.file.rpm)
    args.add("--gpg-key", ctx.file.gpg_key)
    args.add("--content-out", content_tar)
    args.add("--header-out", header_blob)
    args.add("--package", ctx.attr.package)
    args.add("--version", ctx.attr.version)
    args.add("--arch", ctx.attr.arch)

    ctx.actions.run(
        executable = ctx.executable._extract_tool,
        arguments = [args],
        inputs = [ctx.file.rpm, ctx.file.gpg_key],
        outputs = [content_tar, header_blob],
        mnemonic = "RpmExtract",
        progress_message = "Extracting %s-%s (%s)" % (ctx.attr.package, ctx.attr.version, ctx.attr.arch),
    )

    return [
        DefaultInfo(files = depset([content_tar])),
        OutputGroupInfo(
            content = depset([content_tar]),
            header = depset([header_blob]),
        ),
        RpmHeaderInfo(
            header = header_blob,
            package = ctx.attr.package,
            version = ctx.attr.version,
            arch = ctx.attr.arch,
        ),
    ]

rpm_package = rule(
    implementation = _rpm_package_impl,
    attrs = {
        "rpm": attr.label(
            allow_single_file = [".rpm"],
            mandatory = True,
            doc = "Source .rpm file (downloaded by rpm_package_repo).",
        ),
        "gpg_key": attr.label(
            allow_single_file = True,
            mandatory = True,
            doc = "ASCII-armored PGP key used to verify the in-rpm signature.",
        ),
        "package": attr.string(mandatory = True),
        "version": attr.string(mandatory = True),
        "arch": attr.string(mandatory = True),
        "_extract_tool": attr.label(
            default = "@rules_rpm//rpm/tools/rpm-extract",
            executable = True,
            cfg = "exec",
        ),
    },
    doc = "Extracts one .rpm into a content tar (DefaultInfo) and header blob (RpmHeaderInfo).",
)
