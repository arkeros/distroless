"""Module extension for bifrost toolchains."""

load(":versions.bzl", "BIFROST_VERSIONS", "DEFAULT_VERSION", "get_bifrost_url")

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

def _bifrost_repo_impl(rctx):
    version = rctx.attr.version
    platform = _detect_platform(rctx)
    key = version + "-" + platform

    if key not in BIFROST_VERSIONS:
        fail("Bifrost {} not available for {}. Available: {}".format(
            version,
            platform,
            [k for k in BIFROST_VERSIONS.keys() if k.startswith(version)],
        ))

    filename, sha256 = BIFROST_VERSIONS[key]
    url = get_bifrost_url(version, filename)

    rctx.download(
        url = url,
        output = "bifrost",
        sha256 = sha256,
        executable = True,
    )

    constraints = _PLATFORM_CONSTRAINTS[platform]

    rctx.file("BUILD.bazel", """\
load("@rules_shell//shell:sh_binary.bzl", "sh_binary")
load("@senku//devtools/bifrost/toolchain:toolchain.bzl", "bifrost_toolchain")

sh_binary(
    name = "bifrost_bin",
    srcs = ["bifrost"],
    visibility = ["//visibility:public"],
)

bifrost_toolchain(
    name = "toolchain",
    bifrost = ":bifrost_bin",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "bifrost_toolchain",
    toolchain = ":toolchain",
    toolchain_type = "@senku//devtools/bifrost/toolchain:toolchain_type",
    exec_compatible_with = {constraints},
)
""".format(constraints = constraints))

_bifrost_repo = repository_rule(
    implementation = _bifrost_repo_impl,
    attrs = {
        "version": attr.string(mandatory = True),
    },
)

def _bifrost_extension_impl(mctx):
    version = DEFAULT_VERSION
    for mod in mctx.modules:
        for toolchain in mod.tags.toolchain:
            if toolchain.version:
                version = toolchain.version

    _bifrost_repo(name = "bifrost_toolchains", version = version)

bifrost = module_extension(
    implementation = _bifrost_extension_impl,
    tag_classes = {
        "toolchain": tag_class(attrs = {
            "version": attr.string(
                doc = "Bifrost version to use. Defaults to " + DEFAULT_VERSION,
            ),
        }),
    },
)
