load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

package(default_visibility = ["//rpm:__subpackages__"])

bzl_library(
    name = "install",
    srcs = ["install.bzl"],
    deps = [
        ":gpg",
        ":lockfile",
    ],
)

bzl_library(
    name = "per_package",
    srcs = ["per_package.bzl"],
    deps = [":providers"],
)

bzl_library(
    name = "providers",
    srcs = ["providers.bzl"],
)

bzl_library(
    name = "rpmdb_merge",
    srcs = ["rpmdb_merge.bzl"],
    deps = [
        ":gather",
        ":providers",
    ],
)

bzl_library(
    name = "gather",
    srcs = ["gather.bzl"],
    deps = [":providers"],
)

bzl_library(
    name = "lockfile",
    srcs = ["lockfile.bzl"],
)

bzl_library(
    name = "gpg",
    srcs = ["gpg.bzl"],
)

bzl_library(
    name = "pin",
    srcs = ["pin.bzl"],
    deps = ["@bazel_skylib//rules:write_file"],
)

exports_files(["pin.sh.tpl"])
