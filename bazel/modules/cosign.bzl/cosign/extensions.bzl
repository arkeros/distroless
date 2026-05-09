"""Module extension that registers a cosign toolchain from a prebuilt release.

Usage in a consumer's MODULE.bazel:

    bazel_dep(name = "cosign.bzl", version = "0.0.0")
    cosign = use_extension("@cosign.bzl//cosign:extensions.bzl", "cosign")
    use_repo(cosign, "cosign_toolchains")
    register_toolchains("@cosign_toolchains//:all")

Source-compiling cosign from `github.com/sigstore/cosign/v3` instead is
possible but the dep graph fights gazelle (see `bazel/patches/` in senku
for the in-progress workarounds). The prebuilt path is the default.
"""

load(":versions.bzl", "COSIGN_VERSIONS", "DEFAULT_VERSION", "get_cosign_url")

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
        fail("Unsupported OS for cosign prebuilt: " + os)
    if arch in ("aarch64", "arm64"):
        platform_arch = "arm64"
    elif arch in ("x86_64", "amd64"):
        platform_arch = "amd64"
    else:
        fail("Unsupported arch for cosign prebuilt: " + arch)
    return platform_os + "_" + platform_arch

def _cosign_repo_impl(rctx):
    version = rctx.attr.version
    platform = _detect_platform(rctx)
    key = version + "-" + platform
    if key not in COSIGN_VERSIONS:
        fail("Cosign {} not available for {}. Available: {}".format(
            version,
            platform,
            [k for k in COSIGN_VERSIONS.keys() if k.startswith(version)],
        ))
    filename, sha256 = COSIGN_VERSIONS[key]
    url = get_cosign_url(version, filename)

    # Cosign releases are single static binaries — download directly, no archive.
    rctx.download(
        url = url,
        sha256 = sha256,
        output = "cosign",
        executable = True,
    )

    constraints = _PLATFORM_CONSTRAINTS[platform]

    rctx.file("BUILD.bazel", """
load("@rules_shell//shell:sh_binary.bzl", "sh_binary")
load("@cosign.bzl//cosign/toolchain:toolchain.bzl", "cosign_toolchain")

sh_binary(
    name = "cosign_bin",
    srcs = ["cosign"],
    visibility = ["//visibility:public"],
)

cosign_toolchain(
    name = "toolchain",
    cosign = ":cosign_bin",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "cosign_toolchain",
    toolchain = ":toolchain",
    toolchain_type = "@cosign.bzl//cosign:toolchain",
    exec_compatible_with = {constraints},
)
""".format(constraints = constraints))

_cosign_repo = repository_rule(
    implementation = _cosign_repo_impl,
    attrs = {
        "version": attr.string(mandatory = True),
    },
)

def _cosign_extension_impl(mctx):
    version = DEFAULT_VERSION
    for mod in mctx.modules:
        for toolchain in mod.tags.toolchain:
            if toolchain.version:
                version = toolchain.version
    _cosign_repo(name = "cosign_toolchains", version = version)

cosign = module_extension(
    implementation = _cosign_extension_impl,
    tag_classes = {
        "toolchain": tag_class(attrs = {
            "version": attr.string(
                doc = "Cosign version to use. Defaults to " + DEFAULT_VERSION,
            ),
        }),
    },
)
