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
load(
    ":platforms.bzl",
    _PLATFORM_CONSTRAINTS = "PLATFORM_CONSTRAINTS",
    _PROVIDER_PLATFORMS = "PLATFORMS",
)
load(":versions.bzl", "DEFAULT_VERSION", "TERRAFORM_VERSIONS", "get_terraform_url")

# Apparent name of the toolchain repo this extension produces. The hub
# repo's `:pin` script resolves `<this>/terraform_bin` via rlocation,
# which works because the hub repo and the toolchain repo are both
# created by THIS extension and therefore share a repo_mapping that
# resolves `terraform_toolchains` to its canonical name.
#
# Keep in mind if you ever add a per-install toolchain knob (e.g.
# `terraform.install(name=, toolchain=)` to pick different terraform
# versions per install): the script gets `{TOOLCHAIN_RLOC}` substituted
# at hub-repo-rule time, so the change lives here — replace the
# substitution value with whatever the install resolves to.
_TERRAFORM_TOOLCHAIN_REPO = "terraform_toolchains"

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

    Trust model — and its relaxation:

      The lockfile is the only trusted source of hashes. A tampered
      upstream zip would produce a sha256 outside the `zh:` set and
      `fail()` here.

      What this check does NOT catch: if `_provider_url` for platform X
      somehow returned the *wrong but still upstream-blessed* artifact
      — say the `_manifest.json` or a different platform's zip whose
      sha256 also happens to be in the `zh:` set — we'd accept the
      download here, and bazel would fail later (unzip error, terraform
      init mismatch). In other words, this verifies "the downloaded
      bytes were not tampered with mid-flight," not "the URL pointed at
      the file we expected." We trust `_provider_url` to be correct.

      Why we don't tighten: terraform's `.terraform.lock.hcl` lists all
      `zh:` hashes flat — manifest, sig, per-platform zips, cross-
      platform zips — without per-artifact labels. Mapping a `zh:` to
      "this is the linux_arm64 zip's hash" requires fetching
      `SHA256SUMS` separately, which is exactly the custom tooling we
      just deleted. The relaxed check is the cost of staying on the
      terraform-native lockfile format.
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
load("@rules_shell//shell:sh_binary.bzl", "sh_binary")
load("@terraform.bzl//terraform:provider.bzl", "tf_provider_target")

{targets}

# `bazel run @<install>//:pin` shells the registered terraform toolchain
# at `versions.tf` + `.terraform.lock.hcl` and refreshes the lockfile for
# every supported platform. Same effect as `terraform providers lock
# -platform=...` run by hand, but no local terraform required.
sh_binary(
    name = "pin",
    srcs = ["pin.sh"],
    data = ["@terraform_toolchains//:terraform_bin"],
    deps = ["@bazel_tools//tools/bash/runfiles"],
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

    rctx.template(
        "pin.sh",
        rctx.attr._pin_template,
        substitutions = {
            "{LOCK_DIR_REL}": rctx.attr.lock_dir,
            "{TOOLCHAIN_RLOC}": rctx.attr.toolchain_rloc,
        },
        executable = True,
    )

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
        "lock_dir": attr.string(
            mandatory = True,
            doc = "Workspace-relative directory containing the lockfile (e.g. `\"\"` for repo root, `bazel/include` for nested). Baked into `pin.sh` as the `-chdir=$BUILD_WORKSPACE_DIRECTORY/<lock_dir>` argument to terraform.",
        ),
        "toolchain_rloc": attr.string(
            mandatory = True,
            doc = "rlocation key for the terraform binary, e.g. `terraform_toolchains/terraform_bin`. Baked into `pin.sh`. Decouples the script from any single toolchain repo — a future per-install toolchain knob just changes this value.",
        ),
        "_pin_template": attr.label(
            default = "@terraform.bzl//terraform:pin.sh.tpl",
            allow_single_file = True,
        ),
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

    _terraform_repo(name = _TERRAFORM_TOOLCHAIN_REPO, version = version)

    for install in install_tags:
        content = mctx.read(install.lock_file)
        parsed = parse_terraform_lockfile(
            content,
            source = str(install.lock_file),
        ) if content.strip() else {}

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

            # `zh:<hex>` entries are sha256s of every artifact in the
            # SHA256SUMS file: each platform zip, plus the manifest, sig,
            # cross-platform zips, etc. Strip the prefix for the archive
            # repo's verification check — our specific download must be
            # in this set.
            zh_sha256s = [
                h[len("zh:"):]
                for h in hashes
                if h.startswith("zh:")
            ]

            # `h1:<base64>` entries are per-platform dirhashes — one per
            # platform `terraform providers lock -platform=…` was invoked
            # for. This is the actual platform-coverage check: fewer h1
            # entries than supported platforms means someone ran
            # `providers lock` without all `-platform=` flags.
            h1_hashes = [h for h in hashes if h.startswith("h1:")]
            if len(h1_hashes) < len(_PROVIDER_PLATFORMS):
                fail(("terraform.install({install}): provider {source} in " +
                      "{lock} has {got} `h1:` hashes but {want} platforms " +
                      "are supported. Re-run `bazel run @{install}//:pin` " +
                      "(or `terraform providers lock` with all four " +
                      "`-platform=` flags).").format(
                    install = install.name,
                    source = source,
                    lock = install.lock_file,
                    got = len(h1_hashes),
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

        # `lock_dir` = workspace-relative dir of the lockfile. Empty when
        # the lockfile lives at repo root; the senku side is the common
        # case (`//:.terraform.lock.hcl` → package == "" → lock_dir == "").
        _hub_repo(
            name = install.name,
            specs = specs,
            lock_dir = install.lock_file.package,
            toolchain_rloc = _TERRAFORM_TOOLCHAIN_REPO + "/terraform_bin",
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
