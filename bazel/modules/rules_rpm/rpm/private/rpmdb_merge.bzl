"""Aspect-driven `rpmdb_merge` — sbom_rule shape.

`rpmdb_merge_rule(gathering_aspect)` is a factory: pass it the aspect that
collects per-package `RpmHeaderInfo` (default: `gather_rpm_headers`) and it
returns a rule with one `targets` attr. Point `targets` at the image layer(s)
or flatten target(s) whose RPM contents you want catalogued — the aspect
walks each graph, the rule unions the gathered RpmHeaderInfo and emits a tar
containing /usr/lib/sysimage/rpm/rpmdb.sqlite with one Packages row per
unique header.

Image BUILDs look like:

    flatten(name = "static_amd64_layer", tars = [
        "@hummingbird//tzdata/noarch",
        "@hummingbird//ca-certificates/noarch",
        "@hummingbird//mailcap/noarch",
        ...
    ])
    rpmdb_merge(name = "rpmdb_tar", targets = [":static_amd64_layer"])
    oci_image(layers = [":static_amd64_layer", ":rpmdb_tar"], ...)

For a debug image that adds packages on top of a release layer, pass both:

    rpmdb_merge(
        name = "rpmdb_debug_tar",
        targets = [":static_amd64_layer", ":busybox_layer"],
    )

This reuses the existing release flatten as a target rather than re-enumerating
its package set. Duplicate headers (same (package, arch)) are deduplicated.
"""

load(":gather.bzl", "gather_rpm_headers")
load(":providers.bzl", "TransitiveRpmHeaderInfo")

def _rpmdb_merge_impl(ctx):
    seen = {}
    headers = []
    for target in ctx.attr.targets:
        if TransitiveRpmHeaderInfo not in target:
            continue
        for h in target[TransitiveRpmHeaderInfo].headers.to_list():
            key = (h.package, h.arch)
            if key in seen:
                continue
            seen[key] = True
            headers.append(h)

    if not headers:
        # Fail loud rather than emit an empty rpmdb. Shipping an
        # `rpmdb.sqlite` with zero rows would propagate into an image
        # as "scanner reports 0 packages, 0 CVEs" — the AlmaLinux
        # `ID_LIKE` silent-zero trap ADR 0007 disqualifies competitors
        # over. If you actually want an empty rpmdb (one-off CLI use),
        # invoke the rpmdb-merge Go binary directly with an empty
        # config; the rule contract is "build the rpmdb for THESE
        # images" and that contract isn't satisfied by zero inputs.
        fail("rpmdb_merge: no RpmHeaderInfo reachable from targets %s; " % [str(t.label) for t in ctx.attr.targets] +
             "include at least one flatten/layer whose tars contain rpm_package targets " +
             "(e.g. //oci/distroless/static:static_<arch>_hummingbird_layer).")

    # Stable ordering keyed on (package, arch) so the sqlite output is
    # reproducible across builds regardless of aspect traversal order.
    headers = sorted(headers, key = lambda h: (h.package, h.arch))

    config_entries = [
        {
            "package": h.package,
            "version": h.version,
            "arch": h.arch,
            "header_path": h.header.path,
        }
        for h in headers
    ]
    config = ctx.actions.declare_file(ctx.label.name + ".config.json")
    ctx.actions.write(config, json.encode({"headers": config_entries}))

    out = ctx.actions.declare_file(ctx.label.name + ".tar.zst")
    ctx.actions.run(
        executable = ctx.executable._tool,
        arguments = [
            "--config",
            config.path,
            "--out",
            out.path,
            "--compress",
            "zstd",
        ],
        inputs = depset([config] + [h.header for h in headers]),
        outputs = [out],
        mnemonic = "RpmdbMerge",
        progress_message = "Merging rpmdb sqlite for %{label} (" + str(len(headers)) + " headers)",
    )
    return [DefaultInfo(files = depset([out]))]

def rpmdb_merge_rule(gathering_aspect = None):
    """Returns an `rpmdb_merge` rule wired to `gathering_aspect`.

    Args:
      gathering_aspect: aspect emitting TransitiveRpmHeaderInfo. Defaults to
        `gather_rpm_headers` — pass a custom aspect to layer extra filtering
        (e.g. excluding *-debuginfo packages, scoping to specific subtrees).
    """
    aspect = gathering_aspect if gathering_aspect != None else gather_rpm_headers
    return rule(
        implementation = _rpmdb_merge_impl,
        attrs = {
            "targets": attr.label_list(
                aspects = [aspect],
                mandatory = True,
                doc = "Image/flatten targets to harvest RPM headers from. Duplicates dedupe by (package, arch).",
            ),
            "_tool": attr.label(
                default = "@rules_rpm//rpm/tools/rpmdb-merge",
                executable = True,
                cfg = "exec",
            ),
        },
    )

rpmdb_merge = rpmdb_merge_rule()
