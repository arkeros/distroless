"""`rpm_pin`: synthesizes a runnable target inside the hub repo that updates
the lockfile in-place. Invoked as `bazel run @<name>//:pin`.

Implementation is a small genrule-style wrapper around the pin Go binary;
the binary fetches and verifies repomd.xml(.asc), reads primary.xml.gz,
resolves the declared package names across all declared architectures, and
writes the JSON lockfile back to its source-tree path via `BUILD_WORKSPACE_DIRECTORY`.
"""

load("@bazel_skylib//rules:write_file.bzl", "write_file")

def _rpm_pin_impl(ctx):
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._template,
        output = script,
        is_executable = True,
        substitutions = {
            "{TOOL}": ctx.executable._tool.short_path,
            "{REPO_URL}": ctx.attr.repo_url,
            "{GPG_KEY}": ctx.file.gpg_key.short_path,
            "{LOCK_FILE}": ctx.attr.lock_file,
            "{PACKAGES}": ",".join(ctx.attr.packages),
            "{ARCHITECTURES}": ",".join(ctx.attr.architectures),
            "{REPOMD_SIGNATURE}": ctx.attr.repomd_signature,
        },
    )
    return [DefaultInfo(
        executable = script,
        runfiles = ctx.runfiles(files = [ctx.executable._tool, ctx.file.gpg_key]),
    )]

rpm_pin = rule(
    implementation = _rpm_pin_impl,
    executable = True,
    attrs = {
        "repo_url": attr.string(mandatory = True),
        "gpg_key": attr.label(allow_single_file = True, mandatory = True),
        "packages": attr.string_list(mandatory = True),
        "architectures": attr.string_list(mandatory = True),
        "lock_file": attr.string(mandatory = True),
        "repomd_signature": attr.string(
            default = "required",
            values = ["required", "optional"],
            doc = "Lock-time policy for repomd.xml.asc. `required` (default) fails the pin run when the upstream returns HTTP 404 on the detached signature. `optional` warns and continues with TLS-only trust at lock time for that arch — opt-in for upstreams that don't publish a detached repomd signature (e.g. Hummingbird's RHPG snapshot). A *tampered* signature still fails under either policy.",
        ),
        "_tool": attr.label(
            default = "@rules_rpm//rpm/tools/pin",
            executable = True,
            cfg = "exec",
        ),
        "_template": attr.label(
            default = "@rules_rpm//rpm/private:pin.sh.tpl",
            allow_single_file = True,
        ),
    },
)
