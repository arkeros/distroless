"""Module extension `terraform` with two tag classes: `toolchain` + `install`.

Shape mirrors rules_jvm_external / rules_rpm:

  * `terraform.toolchain(version=...)` — workspace-wide TF CLI pin. Downloads
    `terraform_<v>_<os>_<arch>.zip` from `releases.hashicorp.com`, declares
    `tf_toolchain`, registers it via `@terraform_toolchains//:terraform_toolchain`.

  * `terraform.install(name=, providers=, lock_file=)` — one hub repo per
    invocation. Reads pins from the JSON `lock_file`; for each
    `(source, version) × platform` declared in `providers`, instantiates a
    per-platform archive repo (sha256-pinned), then aggregates them in the
    hub as `tf_provider_target`s consumed by `tf_root(providers = […])`.
    The hub also exposes `:pin` — a runnable that regenerates the lockfile
    against the currently-declared `providers` set.

Two `install(...)` calls produce two hub repos; consumers can keep
provider sets segregated (e.g. core vs. test-only) without colliding.
"""

load(":versions.bzl", "DEFAULT_VERSION", "TERRAFORM_VERSIONS", "get_terraform_url")

# Provider platforms we build hub-repo entries for. Mirrored in
# `//terraform:provider.bzl::PLATFORMS` (kept side-by-side rather than
# `load`-imported because a module extension cannot load the same .bzl
# as a regular rule does without dragging in transitive load-time
# concerns).
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

    Source layout is `<namespace>/<type>` (e.g. `hashicorp/google`);
    the URL only embeds the type, not the namespace. The release host
    serves zips for all of HashiCorp's official providers under this
    path; third-party namespaces would need a different scheme.
    """
    _, ptype = source.split("/")
    return "https://releases.hashicorp.com/terraform-provider-{ptype}/{version}/terraform-provider-{ptype}_{version}_{platform}.zip".format(
        ptype = ptype,
        version = version,
        platform = platform,
    )

def _provider_archive_repo_impl(rctx):
    """Download one provider zip for one platform; expose it verbatim as a
    `:files` filegroup. The hub repo symlinks it into the per-root mirror
    tree as a packed `.zip` (terraform's filesystem mirror accepts both
    packed and unpacked layouts; we use packed because the lockfile h1
    verification is computed against the zip by `dirhash.HashZip` and
    matches what terraform's own packed-mirror code path expects
    byte-for-byte. The unpacked layout requires terraform to re-derive h1
    from the extracted file, which has proven brittle in practice — the
    same provider+version produces different h1 strings between
    `dirhash.HashZip` and a re-extraction).
    """
    _, ptype = rctx.attr.source.split("/")
    zip_name = "terraform-provider-{ptype}_{version}_{platform}.zip".format(
        ptype = ptype,
        version = rctx.attr.version,
        platform = rctx.attr.platform,
    )
    rctx.download(
        url = _provider_url(rctx.attr.source, rctx.attr.version, rctx.attr.platform),
        output = zip_name,
        sha256 = rctx.attr.sha256,
    )
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
        "sha256": attr.string(mandatory = True),
    },
)

def _archive_repo_name(install_name, source, version, platform):
    """Repo name for the per-platform archive of one provider.

    The (install, source, version, platform) tuple is embedded in the
    name on purpose — it's part of the cache-invalidation contract, not
    just a uniqueness key. Bumping a version intentionally produces a
    *new* repo name, which makes Bazel's repo cache treat the new
    archive as a wholly fresh entity and re-run `download` against the
    new sha256.
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
load("@rules_go//go:def.bzl", "go_binary")

{targets}

# `bazel run @{install}//:pin` regenerates the lockfile against the
# currently-declared provider set. The lockfile path + provider list
# are baked into `args` at extension-evaluation time, so the consumer
# never sees a per-install flag.
go_binary(
    name = "pin",
    embed = ["@terraform.bzl//terraform/private/pin:pin_lib"],
    args = [
        {args}
    ],
    visibility = ["//visibility:public"],
)
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
    hashes = {hashes},
    archives = {archives},
    visibility = ["//visibility:public"],
)
""".format(
            name = repr(spec["name"]),
            source = repr(spec["source"]),
            version = repr(spec["version"]),
            hashes = repr(spec["hashes"]),
            archives = repr(spec["archives"]),
        ))

    # `args` for the pin go_binary. Workspace-relative path + each
    # declared `<source>@<version>` — fully resolved at extension time,
    # so the pin tool takes flat flags and doesn't need to understand
    # bzlmod.
    args = ['"-lock={}"'.format(rctx.attr.lock_path)]
    for spec in rctx.attr.provider_keys:
        args.append('"-provider={}"'.format(spec))

    rctx.file("BUILD.bazel", _HUB_BUILD_TEMPLATE.format(
        install = rctx.attr.install_name,
        targets = "\n".join(targets),
        args = ",\n        ".join(args),
    ))

_hub_repo = repository_rule(
    implementation = _hub_repo_impl,
    attrs = {
        "install_name": attr.string(mandatory = True),
        # JSON-encoded specs (one per provider) with name/source/version/
        # hashes/archives. JSON used as the wire format because rctx attrs
        # are flat — can't hold dict-of-dict.
        "specs": attr.string_list(mandatory = True),
        "lock_path": attr.string(
            mandatory = True,
            doc = "Workspace-relative path of the JSON lockfile, e.g. `bazel/include/terraform.providers.lock.json`. The pin tool resolves it under `$BUILD_WORKSPACE_DIRECTORY`.",
        ),
        "provider_keys": attr.string_list(
            mandatory = True,
            doc = "`<source>@<version>` strings — the declared provider set; one `-provider=` flag per entry.",
        ),
    },
)

# ---------- top-level extension --------------------------------------------

def _parse_lock_json(mctx, lock_file_label):
    """Read the JSON lockfile via `mctx.read` and return the providers map.

    Lockfile shape:
        {
          "providers": {
            "hashicorp/google@7.29.0": {
              "darwin_amd64": {"sha256": "...", "h1": "h1:..."},
              ...
            },
            ...
          }
        }

    Missing file → empty map (bootstrap: pin tool hasn't run yet). Missing
    individual providers in an otherwise-valid file → also empty for that
    key, which surfaces as a per-provider warning + skip below.
    """
    body = mctx.read(lock_file_label)
    if not body.strip():
        return {}
    parsed = json.decode(body)
    return parsed.get("providers", {})

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
        per_provider_hashes = _parse_lock_json(mctx, install.lock_file)
        specs = []

        for source, ver in install.providers.items():
            key = "{}@{}".format(source, ver)
            per_platform = per_provider_hashes.get(key)
            if per_platform == None:
                # buildifier: disable=print
                print(("WARNING: terraform.install({}): provider {} not " +
                       "pinned in {}. Run `bazel run @{}//:pin`.").format(
                    install.name,
                    key,
                    install.lock_file,
                    install.name,
                ))
                continue

            hashes_h1 = {}
            archives = {}
            for platform in _PROVIDER_PLATFORMS:
                if platform not in per_platform:
                    fail("provider {} has no pin for platform {}".format(key, platform))
                entry = per_platform[platform]
                archive_name = _archive_repo_name(install.name, source, ver, platform)
                _provider_archive_repo(
                    name = archive_name,
                    source = source,
                    version = ver,
                    platform = platform,
                    sha256 = entry["sha256"],
                )
                archives[platform] = "@{}//:files".format(archive_name)
                hashes_h1[platform] = entry["h1"]

            specs.append(json.encode({
                "name": _short_name(source),
                "source": source,
                "version": ver,
                "hashes": hashes_h1,
                "archives": archives,
            }))

        lock_path = install.lock_file.package + "/" + install.lock_file.name
        if not install.lock_file.package:
            lock_path = install.lock_file.name
        _hub_repo(
            name = install.name,
            install_name = install.name,
            specs = specs,
            lock_path = lock_path,
            provider_keys = [
                "{}@{}".format(s, v)
                for s, v in install.providers.items()
            ],
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
            "providers": attr.string_dict(
                mandatory = True,
                doc = "`{source: version}` — e.g. `{\"hashicorp/google\": \"7.29.0\"}`. No constraints; bumps go through the pin tool.",
            ),
            "lock_file": attr.label(
                mandatory = True,
                allow_single_file = [".json"],
                doc = "Path to the committed JSON lockfile. Regenerated by `bazel run @<name>//:pin`.",
            ),
        }),
    },
)
