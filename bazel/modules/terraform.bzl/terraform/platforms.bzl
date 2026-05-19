"""Canonical platform list + bazel constraint mapping for the terraform
module.

`PLATFORMS` is the set of `<os>_<arch>` keys that show up everywhere in
this codebase: as `-platform=` flags to `terraform providers lock`, as
the suffix in `releases.hashicorp.com/.../terraform-provider-<type>_<v>_<plat>.zip`
URLs, as the key set in `archives = {...}` dicts on `tf_provider_target`,
and as keys in `PLATFORM_CONSTRAINTS` below.

`PLATFORM_CONSTRAINTS` is the bazel-side counterpart: each platform name
maps to the `@platforms//os:...` + `@platforms//cpu:...` labels needed
for `exec_compatible_with` on the registered terraform toolchain.

Single source of truth — `extensions.bzl`, `provider.bzl`, and
`lockfile.bzl` all load from here rather than re-declaring.
"""

PLATFORMS = ["darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"]

PLATFORM_CONSTRAINTS = {
    "darwin_amd64": ["@platforms//os:macos", "@platforms//cpu:x86_64"],
    "darwin_arm64": ["@platforms//os:macos", "@platforms//cpu:arm64"],
    "linux_amd64": ["@platforms//os:linux", "@platforms//cpu:x86_64"],
    "linux_arm64": ["@platforms//os:linux", "@platforms//cpu:arm64"],
}
