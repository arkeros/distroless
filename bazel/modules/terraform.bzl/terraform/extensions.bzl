"""Module extension `terraform` with two tag classes: `toolchain` + `install`.

Shape mirrors rules_jvm_external / rules_rpm:

  * `terraform.toolchain(version=...)` — workspace-wide TF CLI pin. Downloads
    `terraform_<v>_<os>_<arch>.zip` from `releases.hashicorp.com`, declares
    `tf_toolchain`, registers it via `@terraform_toolchains//:terraform_toolchain`.

  * `terraform.install(name=, lock_file=)` — one hub repo per invocation.
    `lock_file` points at a `.terraform.lock.hcl` (the file `terraform
    providers lock` writes). Every provider in that lockfile gets a
    `tf_provider_target` in the hub, with per-platform archive repos
    downloading the bytes that match the lockfile's `zh:` hashes.

The lockfile is the single source of truth — both for which providers
exist and for their resolved versions + hashes. Updates flow through
the standard terraform workflow:

  cd <dir with versions.tf + .terraform.lock.hcl>
  terraform providers lock -platform=darwin_amd64 \\
                            -platform=darwin_arm64 \\
                            -platform=linux_amd64 \\
                            -platform=linux_arm64

Renovate's `terraform-lockfile` manager runs that command natively when
a bump appears in a `versions.tf` it discovers, so the day-to-day
workflow is just "merge Renovate PRs".
"""

load(":hcl.bzl", "parse_terraform_lockfile")
load(":versions.bzl", "DEFAULT_VERSION", "TERRAFORM_VERSIONS", "get_terraform_url")

# Provider platforms we materialize archive repos for. Mirrored in
# `//terraform:provider.bzl::PLATFORMS`.
_PROVIDER_PLATFORMS = ["darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"]

_PLATFORM_CONSTRAINTS = {
    "darwin_amd64": ["@platforms//os:macos", "@platforms//cpu:x86_64"],
    "darwin_arm64": ["@platforms//os:macos", "@platforms//cpu:arm64"],
    "linux_amd64": ["@platforms//os:linux", "@platforms//cpu:x86_64"],
    "linux_arm64": ["@platforms//os:linux", "@platforms//cpu:arm64"],
}

def _detect_platform(rctx):
    os = rctx.os.name.lower()
    arch = rctx.os.arch

    if "mac" in os or "darwin" in os:
        platform_os = "darwin"
    elif "linux" in os:
        platform_os = "linux"
    else:
        fail("Unsupported OS: " + os)

    if arch in ("aarch64", "arm64"):
        platform_arch = "arm64"
    elif arch in ("x86_64", "amd64"):
        platform_arch = "amd64"
    else:
        fail("Unsupported arch: " + arch)

    return platform_os + "_" + platform_arch

# ---------- toolchain repo --------------------------------------------------

def _terraform_repo_impl(rctx):
    version = rctx.attr.version
    platform = _detect_platform(rctx)
    key = version + "-" + platform

    if key not in TERRAFORM_VERSIONS:
        fail("Terraform {} not available for {}. Available: {}".format(
            version,
            platform,
            [k for k in TERRAFORM_VERSIONS.keys() if k.startswith(version)],
        ))

    filename, sha256 = TERRAFORM_VERSIONS[key]
    url = get_terraform_url(version, filename)

    rctx.download_and_extract(
        url = url,
        sha256 = sha256,
        type = "zip",
    )

    constraints = _PLATFORM_CONSTRAINTS[platform]

    rctx.file("BUILD.bazel", """\
load("@bazel_skylib//rules:native_binary.bzl", "native_binary")
load("@terraform.bzl//terraform/toolchain:toolchain.bzl", "tf_toolchain")

native_binary(
    name = "terraform_bin",
    src = "terraform",
    out = "terraform_bin",
    visibility = ["//visibility:public"],
)

tf_toolchain(
    name = "toolchain",
    terraform = ":terraform_bin",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "terraform_toolchain",
    toolchain = ":toolchain",
    toolchain_type = "@terraform.bzl//terraform/toolchain:toolchain_type",
    exec_compatible_with = {constraints},
)
""".format(constraints = constraints))

_terraform_repo = repository_rule(
    implementation = _terraform_repo_impl,
    attrs = {
        "version": attr.string(mandatory = True),
    },
)

# ---------- per-platform provider archive repo -----------------------------

def _provider_url(source, version, platform):
    """HashiCorp release URL for a provider archive.

    Source layout is `<namespace>/<type>` (e.g. `hashicorp/google`); the
    URL only embeds the type, not the namespace. The release host serves
    zips for all of HashiCorp's official providers under this path;
    third-party namespaces would need a different scheme.
    """
    _, ptype = source.split("/")
    return "https://releases.hashicorp.com/terraform-provider-{ptype}/{version}/terraform-provider-{ptype}_{version}_{platform}.zip".format(
        ptype = ptype,
        version = version,
        platform = platform,
    )

def _provider_archive_repo_impl(rctx):
    """Download one provider zip for one platform; verify its sha256
    appears in the lockfile's `zh:` set; expose it as a `:files`
    filegroup.

    No `sha256 =` arg on `rctx.download`: we don't know the per-platform
    sha256 directly from the lockfile (terraform's `.terraform.lock.hcl`
    lists all `zh:` hashes flat, not mapped by platform). Instead, we
    download, take the resulting sha256 from `rctx.download`'s return
    value, and assert it's one of the lockfile's `zh:` hashes. This is
    the same trust posture: the lockfile is authoritative, and a
    tampered upstream zip would produce a sha256 outside the trusted
    set.
    """
    _, ptype = rctx.attr.source.split("/")
    zip_name = "terraform-provider-{ptype}_{version}_{platform}.zip".format(
        ptype = ptype,
        version = rctx.attr.version,
        platform = rctx.attr.platform,
    )
    result = rctx.download(
        url = _provider_url(rctx.attr.source, rctx.attr.version, rctx.attr.platform),
        output = zip_name,
    )
    if result.sha256 not in rctx.attr.valid_zh_sha256s:
        fail(("downloaded {zip} has sha256 {got} which does not appear in " +
              "the lockfile's `zh:` set for {source} {version}. Lockfile " +
              "is stale or upstream zip changed without a re-pin.").format(
            zip = zip_name,
            got = result.sha256,
            source = rctx.attr.source,
            version = rctx.attr.version,
        ))
    rctx.file("BUILD.bazel", """\
filegroup(
    name = "files",
    srcs = ["{zip}"],
    visibility = ["//visibility:public"],
)
""".format(zip = zip_name))

_provider_archive_repo = repository_rule(
    implementation = _provider_archive_repo_impl,
    attrs = {
        "source": attr.string(mandatory = True),
        "version": attr.string(mandatory = True),
        "platform": attr.string(mandatory = True),
        "valid_zh_sha256s": attr.string_list(
            mandatory = True,
            doc = "Hex-encoded sha256 values from the lockfile's `zh:` entries (prefix stripped). The downloaded zip's sha256 must be in this set.",
        ),
    },
)

def _archive_repo_name(install_name, source, version, platform):
    """Repo name for the per-platform archive of one provider.

    The (install, source, version, platform) tuple is embedded in the
    name on purpose — it's part of the cache-invalidation contract, not
    just a uniqueness key. Bumping a version intentionally produces a
    *new* repo name so bazel's repo cache treats the new archive as a
    wholly fresh entity.
    """
    return "{install}__provider_{src}_{ver}_{plat}".format(
        install = install_name,
        src = source.replace("/", "_"),
        ver = version.replace(".", "_"),
        plat = platform,
    )

def _short_name(source):
    """`hashicorp/google` → `google`. Matches `provider.bzl::_short_name`."""
    return source.split("/")[-1]

# ---------- hub repo (one per install) --------------------------------------

_HUB_BUILD_TEMPLATE = """\
load("@terraform.bzl//terraform:provider.bzl", "tf_provider_target")

{targets}
"""

def _hub_repo_impl(rctx):
    targets = []
    for spec_json in rctx.attr.specs:
        spec = json.decode(spec_json)
        targets.append("""\
tf_provider_target(
    name = {name},
    source = {source},
    version = {version},
    constraints = {constraints},
    hashes = {hashes},
    archives = {archives},
    visibility = ["//visibility:public"],
)
""".format(
            name = repr(spec["name"]),
            source = repr(spec["source"]),
            version = repr(spec["version"]),
            constraints = repr(spec["constraints"]),
            hashes = repr(spec["hashes"]),
            archives = repr(spec["archives"]),
        ))

    rctx.file("BUILD.bazel", _HUB_BUILD_TEMPLATE.format(
        targets = "\n".join(targets),
    ))

_hub_repo = repository_rule(
    implementation = _hub_repo_impl,
    attrs = {
        # JSON-encoded specs (one per provider): name/source/version/
        # hashes/archives. JSON is the wire format because rctx attrs
        # are flat — can't hold dict-of-dict.
        "specs": attr.string_list(mandatory = True),
    },
)

# ---------- top-level extension --------------------------------------------

def _terraform_extension_impl(mctx):
    version = DEFAULT_VERSION
    install_tags = []
    for mod in mctx.modules:
        for toolchain in mod.tags.toolchain:
            if toolchain.version:
                version = toolchain.version
        for install in mod.tags.install:
            install_tags.append(install)

    _terraform_repo(name = "terraform_toolchains", version = version)

    for install in install_tags:
        content = mctx.read(install.lock_file)
        parsed = parse_terraform_lockfile(content) if content.strip() else {}

        specs = []
        for source, block in parsed.items():
            ver = block.get("version")
            if not ver:
                fail("terraform.install({}): provider {} in {} has no version".format(
                    install.name,
                    source,
                    install.lock_file,
                ))

            hashes = block.get("hashes", [])

            # `zh:<hex>` entries are the per-platform sha256s of the zip.
            # Strip the prefix for the archive repo's verification check.
            zh_sha256s = [
                h[len("zh:"):]
                for h in hashes
                if h.startswith("zh:")
            ]
            if len(zh_sha256s) < len(_PROVIDER_PLATFORMS):
                fail(("terraform.install({install}): provider {source} in " +
                      "{lock} has {got} `zh:` hashes but {want} platforms are " +
                      "supported. Re-run `terraform providers lock` with " +
                      "all four `-platform=` flags.").format(
                    install = install.name,
                    source = source,
                    lock = install.lock_file,
                    got = len(zh_sha256s),
                    want = len(_PROVIDER_PLATFORMS),
                ))

            archives = {}
            for platform in _PROVIDER_PLATFORMS:
                archive_name = _archive_repo_name(install.name, source, ver, platform)
                _provider_archive_repo(
                    name = archive_name,
                    source = source,
                    version = ver,
                    platform = platform,
                    valid_zh_sha256s = zh_sha256s,
                )
                archives[platform] = "@{}//:files".format(archive_name)

            specs.append(json.encode({
                "name": _short_name(source),
                "source": source,
                "version": ver,
                "constraints": block.get("constraints", ""),
                "hashes": hashes,
                "archives": archives,
            }))

        _hub_repo(
            name = install.name,
            specs = specs,
        )

terraform = module_extension(
    implementation = _terraform_extension_impl,
    tag_classes = {
        "toolchain": tag_class(attrs = {
            "version": attr.string(
                doc = "Terraform version to use. Defaults to " + DEFAULT_VERSION,
            ),
        }),
        "install": tag_class(attrs = {
            "name": attr.string(
                mandatory = True,
                doc = "Hub repo name. Consumers `use_repo(terraform, \"<name>\")` to expose it.",
            ),
            "lock_file": attr.label(
                mandatory = True,
                allow_single_file = [".terraform.lock.hcl"],
                doc = "A terraform-native `.terraform.lock.hcl`. Generated by `terraform providers lock` against a sibling `versions.tf`; bumps are handled by Renovate's `terraform-lockfile` manager.",
            ),
        }),
    },
)
