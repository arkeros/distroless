"""Module extension for terraform toolchains.

Downloads + extracts the official `terraform_<version>_<os>_<arch>.zip`
archive from `https://releases.hashicorp.com/terraform/`, then declares
a `tf_toolchain` and registers it. The archive contains a single
`terraform` executable at the archive root.
"""

load(":versions.bzl", "DEFAULT_VERSION", "TERRAFORM_VERSIONS", "get_terraform_url")

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
load("@rules_shell//shell:sh_binary.bzl", "sh_binary")
load("@senku//devtools/build/tools/tf/toolchain:toolchain.bzl", "tf_toolchain")

sh_binary(
    name = "terraform_bin",
    srcs = ["terraform"],
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
    toolchain_type = "@senku//devtools/build/tools/tf/toolchain:toolchain_type",
    exec_compatible_with = {constraints},
)
""".format(constraints = constraints))

_terraform_repo = repository_rule(
    implementation = _terraform_repo_impl,
    attrs = {
        "version": attr.string(mandatory = True),
    },
)

def _terraform_extension_impl(mctx):
    version = DEFAULT_VERSION
    for mod in mctx.modules:
        for toolchain in mod.tags.toolchain:
            if toolchain.version:
                version = toolchain.version

    _terraform_repo(name = "terraform_toolchains", version = version)

terraform = module_extension(
    implementation = _terraform_extension_impl,
    tag_classes = {
        "toolchain": tag_class(attrs = {
            "version": attr.string(
                doc = "Terraform version to use. Defaults to " + DEFAULT_VERSION,
            ),
        }),
    },
)
