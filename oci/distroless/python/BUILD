load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@rules_distroless//distroless:defs.bzl", "flatten")
load(":config.bzl", "PYTHON_ARCHITECTURES", "PYTHON_DISTROS", "PYTHON_PACKAGES")

package(default_visibility = ["//visibility:public"])

[
    flatten(
        name = arch + "_" + distro + "_layer",
        deduplicate = True,
        tars = [
            "@{}//{}/{}".format(distro, pkg, arch)
            for pkg in PYTHON_PACKAGES
        ],
    )
    for distro in PYTHON_DISTROS
    for arch in PYTHON_ARCHITECTURES[distro]
]

bzl_library(
    name = "config",
    srcs = ["config.bzl"],
)
