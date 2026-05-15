"""Aspect that gathers RpmHeaderInfo across the transitive deps of an image.

Shape mirrors @supply_chain_tools//tools/gather_metadata:gather_metadata.bzl
but kept deliberately minimal — we don't need edge tracking, license filtering,
or exec-config heuristics here. rpm_package targets are leaves of the package
graph; the aspect just walks every label-shaped attr (`attr_aspects = ["*"]`)
collecting RpmHeaderInfo as it goes.

Used by rpmdb_merge_rule (see private/rpmdb_merge.bzl): the rule's `target`
attribute carries this aspect, so any image label is enough to harvest its
entire RPM header set without consumers maintaining a parallel
`headers = [...]` enumeration.
"""

load(":providers.bzl", "RpmHeaderInfo", "TransitiveRpmHeaderInfo")

def _gather_rpm_headers_impl(target, ctx):
    direct = []
    if RpmHeaderInfo in target:
        direct.append(target[RpmHeaderInfo])

    transitive = []
    for attr_name in dir(ctx.rule.attr):
        if attr_name.startswith("_"):
            continue
        attr_value = getattr(ctx.rule.attr, attr_name, None)
        deps = attr_value if type(attr_value) == type([]) else [attr_value]
        for dep in deps:
            if type(dep) == "Target" and TransitiveRpmHeaderInfo in dep:
                transitive.append(dep[TransitiveRpmHeaderInfo].headers)

    return [TransitiveRpmHeaderInfo(
        headers = depset(direct = direct, transitive = transitive),
    )]

gather_rpm_headers = aspect(
    doc = "Gathers RpmHeaderInfo from all rpm_package targets reachable from the root.",
    implementation = _gather_rpm_headers_impl,
    attr_aspects = ["*"],
    provides = [TransitiveRpmHeaderInfo],
    apply_to_generating_rules = True,
)
