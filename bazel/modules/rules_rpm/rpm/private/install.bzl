"""Two repository_rules: per-package spoke and hub.

Spoke (`rpm_package_repo`): downloads one .rpm from `url`, verifies sha256
and the embedded GPG signature, then runs `rpm-extract` to produce two
labels:
  - `:content`  → tar of allow-listed file paths with canonical uid/gid/mtime
  - `:header`   → raw RPM header blob (input to rpmdb_merge)

Hub (`rpm_install_repo`): an aliases-only repo. Re-exports every spoke under
`@<name>//<pkg>/<arch>:content` and `:header`, plus a `:pin` runnable that
shells out to the pin Go binary against the live repo.
"""

# Bazel repo names must match `[A-Za-z0-9._-]+`. RPM package names use `+`
# freely (libstdc++, gtk+, ...). We map `+` -> `.plus.` for the spoke repo
# name only; the hub-side alias under `@<hub>//<pkg>/<arch>` keeps the
# original `+` (Bazel package directories accept it). Both ends of the
# aliasing must agree, hence the public helper.
def safe_repo_name(s):
    return s.replace("+", ".plus.")

def _split_epoch(evr):
    # RPM versions optionally carry an epoch prefix (`<epoch>:<ver>-<rel>`).
    # purl-spec routes epoch into its own qualifier rather than baking it
    # into the version field, so split here. Falls back to "0" for the
    # implicit-epoch case.
    if ":" in evr:
        epoch, _, rest = evr.partition(":")
        return epoch, rest
    return "0", evr

def _rpm_purl(namespace, name, version, arch, upstream):
    # purl-spec for rpm: pkg:rpm/<namespace>/<name>@<ver>-<rel>?<qualifiers>
    # `namespace` is the upstream/distro identity (e.g. "hummingbird",
    # "nginx.org"). epoch + arch + upstream are encoded as qualifiers so
    # the purl is purl-spec-conformant and interop-compatible with syft's
    # rpmdb-cataloged shape (`epoch=N&arch=X&upstream=<src.rpm>`).
    epoch, ver = _split_epoch(version)
    parts = [
        "arch=" + arch,
        "epoch=" + epoch,
    ]
    if upstream:
        parts.append("upstream=" + upstream)
    return "pkg:rpm/{ns}/{name}@{ver}?{q}".format(
        ns = namespace,
        name = name,
        ver = ver,
        q = "&".join(parts),
    )

def _rpm_package_repo_impl(rctx):
    # Repository rules run before the analysis phase, so we can't invoke the
    # `rpm-extract` go_binary here (it's not built yet). The repo only
    # downloads the .rpm; extraction is deferred to an `rpm_package` rule
    # whose action depends on the go_binary as a regular tool input.
    rctx.download(
        url = rctx.attr.url,
        sha256 = rctx.attr.sha256,
        output = "package.rpm",
    )

    purl = _rpm_purl(
        namespace = rctx.attr.purl_namespace,
        name = rctx.attr.package,
        version = rctx.attr.version,
        arch = rctx.attr.arch,
        upstream = rctx.attr.upstream,
    )

    # `package_metadata(purl=...)` + `package(default_package_metadata=...)`
    # surfaces the rpm's identity to supply_chain_tools' gather_metadata
    # aspect. Without this, the image SBOM lists rpm-extract's Go module deps
    # as components instead of the actual rpm packages — see
    # //oci/distroless/common:package.BUILD.tmpl for the apt-side analogue.
    rctx.file("BUILD.bazel", """
load("@package_metadata//:defs.bzl", "package_metadata")
load("@rules_rpm//rpm/private:per_package.bzl", "rpm_package")

package(
    default_package_metadata = [":package_metadata"],
    default_visibility = ["//visibility:public"],
)

exports_files(["package.rpm"])

package_metadata(
    name = "package_metadata",
    purl = {purl},
    visibility = ["//visibility:public"],
)

rpm_package(
    name = "package",
    rpm = "package.rpm",
    gpg_key = {gpg_key},
    package = {package},
    version = {version},
    arch = {arch},
)
""".format(
        package = repr(rctx.attr.package),
        version = repr(rctx.attr.version),
        arch = repr(rctx.attr.arch),
        gpg_key = repr(str(rctx.attr.gpg_key)),
        purl = repr(purl),
    ))

rpm_package_repo = repository_rule(
    implementation = _rpm_package_repo_impl,
    attrs = {
        "package": attr.string(mandatory = True),
        "version": attr.string(mandatory = True),
        "arch": attr.string(mandatory = True),
        "url": attr.string(mandatory = True),
        "sha256": attr.string(mandatory = True),
        "gpg_key": attr.label(mandatory = True, allow_single_file = True),
        "purl_namespace": attr.string(
            mandatory = True,
            doc = "purl namespace identifying the upstream (e.g. 'hummingbird', 'nginx.org'). Drives the `pkg:rpm/<namespace>/<name>` shape grype routes by.",
        ),
        "upstream": attr.string(
            doc = "Source-rpm filename from primary.xml's <rpm:sourcerpm>. Embedded as `?upstream=...` qualifier in the purl for provenance; optional, omitted when unset.",
        ),
    },
)

def _rpm_install_repo_impl(rctx):
    # Synthesize aliases at @<name>//<pkg>/<arch>:{package,content,header} that
    # point at the spoke repos created by the extension impl. `:package` is the
    # canonical label — it carries DefaultInfo (content tar) and RpmHeaderInfo,
    # so the same label works in flatten(tars=[...]) AND is reachable by
    # gather_rpm_headers for rpmdb_merge.
    arches = rctx.attr.architectures + ["noarch"]
    for pkg in rctx.attr.packages:
        for arch in arches:
            spoke = "@@{}__{}__{}".format(rctx.attr.name, safe_repo_name(pkg), arch)
            rctx.file(
                "%s/%s/BUILD.bazel" % (pkg, arch),
                """
package(default_visibility = ["//visibility:public"])

# Default target named after the directory (`{arch}`) so consumers can use
# the bare label `@<name>//<pkg>/<arch>` — same convention as
# rules_distroless's @debian//pkg/arch and rules_jvm_external repos.
alias(name = "{arch}",  actual = "{spoke}//:package")
alias(name = "package", actual = "{spoke}//:package")
""".format(arch = arch, spoke = spoke),
            )

    # Stub aliases for manifest roots not present in the lockfile (= user
    # edited `packages` without running pin). The stub's BUILD.bazel is
    # only loaded when something queries the missing label — so
    # `bazel run @<name>//:pin` (the remediation tool) keeps working, but
    # `bazel build //consumer/of/missing` surfaces a helpful error instead
    # of Bazel's bare "no such target".
    lock_keys = {pkg: True for pkg in rctx.attr.packages}
    for pkg in rctx.attr.package_list:
        if pkg in lock_keys:
            continue
        for arch in arches:
            rctx.file(
                "%s/%s/BUILD.bazel" % (pkg, arch),
                'fail("rules_rpm: lockfile is stale — manifest root \\"{pkg}\\" is not in the lockfile. Run `bazel run @{name}//:pin` to refresh.")\n'.format(
                    pkg = pkg,
                    name = rctx.attr.name,
                ),
            )

    # The :pin runnable. `bazel run @<name>//:pin -- --update` walks the live
    # repo, resolves the declared package list, and rewrites the lockfile.
    rctx.file("BUILD.bazel", """
load("@rules_rpm//rpm/private:pin.bzl", "rpm_pin")

package(default_visibility = ["//visibility:public"])

rpm_pin(
    name = "pin",
    repo_url = {repo_url},
    gpg_key = {gpg_key},
    packages = {packages},
    architectures = {architectures},
    lock_file = {lock_file},
)
""".format(
        repo_url = repr(rctx.attr.repo_url),
        gpg_key = repr(str(rctx.attr.gpg_key)),
        packages = repr(rctx.attr.package_list),
        architectures = repr(rctx.attr.architectures),
        lock_file = repr(str(rctx.attr.lock_file)),
    ))

rpm_install_repo = repository_rule(
    implementation = _rpm_install_repo_impl,
    attrs = {
        "packages": attr.string_list(mandatory = True),
        "architectures": attr.string_list(mandatory = True),
        "package_list": attr.string_list(mandatory = True),
        "repo_url": attr.string(mandatory = True),
        "gpg_key": attr.label(mandatory = True, allow_single_file = True),
        "lock_file": attr.label(mandatory = True, allow_single_file = True),
    },
)
