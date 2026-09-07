"""Module extension `tarballs`: upstream prebuilt runtime tarballs pinned by a lockfile.

One `lock(...)` tag reads a JSON lockfile written by `knife tarballs update`
and declares one `http_archive` per entry of every line's `archives`, named
by the entry's key, so image BUILD files reference e.g. `@nodejs_24_amd64`.
It also generates a hub repo (the tag's `name`) exporting `versions.bzl`
with the lines' versions, which the image's config.bzl loads to build the
SBOM identity (purl + CPE) without repeating the version by hand.

Lockfile schema (`<runtime>.lock.json`):

    {
      "schema_version": 1,
      "source": "nodejs" | "temurin",   # resolver knife uses to refresh it
      "lines": [
        {
          "major": "24",
          "version": "24.19.0",
          "build": "8",                 # upstream build counter; absent for nodejs
          "archives": {
            "<repo name>": {"url": "...", "sha256": "...", "strip_prefix": "..."}
          }
        }
      ]
    }

Which majors a lockfile lists is a policy decision made by hand in that
file; the updater only moves listed lines to their newest release.
"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

_lock = tag_class(
    attrs = {
        "name": attr.string(
            mandatory = True,
            doc = "Hub repo name. `load(\"@<name>//:versions.bzl\", \"VERSIONS\")` yields the lockfile's lines as a list of {major, version, build} dicts, in file order.",
        ),
        "lockfile": attr.label(
            mandatory = True,
            allow_single_file = [".json"],
            doc = "The committed lockfile. Refresh with `knife tarballs update <path>`.",
        ),
        "build_file_content": attr.string(
            mandatory = True,
            doc = "BUILD file content for every archive of this lockfile.",
        ),
    },
)

def _versions_repo_impl(rctx):
    rctx.file("BUILD.bazel", """\
load("@bazel_skylib//:bzl_library.bzl", "bzl_library")

bzl_library(
    name = "versions",
    srcs = ["versions.bzl"],
    visibility = ["//visibility:public"],
)
""")
    rctx.file("versions.bzl", "VERSIONS = json.decode({})\n".format(repr(rctx.attr.versions_json)))

_versions_repo = repository_rule(
    implementation = _versions_repo_impl,
    attrs = {"versions_json": attr.string(mandatory = True)},
)

def _tarballs_impl(mctx):
    for mod in mctx.modules:
        for lock in mod.tags.lock:
            parsed = json.decode(mctx.read(lock.lockfile))
            if parsed.get("schema_version") != 1:
                fail("tarballs: {} has unsupported schema_version {}".format(lock.lockfile, parsed.get("schema_version")))
            for line in parsed["lines"]:
                for repo_name, archive in line["archives"].items():
                    http_archive(
                        name = repo_name,
                        url = archive["url"],
                        sha256 = archive["sha256"],
                        strip_prefix = archive["strip_prefix"],
                        build_file_content = lock.build_file_content,
                    )
            _versions_repo(
                name = lock.name,
                versions_json = json.encode([{
                    "major": line["major"],
                    "version": line["version"],
                    "build": line.get("build", ""),
                } for line in parsed["lines"]]),
            )

    # Every repo is a direct dep of the root module (the image BUILD files
    # reference them by name), so `bazel mod tidy` keeps use_repo complete.
    return mctx.extension_metadata(
        root_module_direct_deps = "all",
        root_module_direct_dev_deps = [],
        reproducible = True,
    )

tarballs = module_extension(
    implementation = _tarballs_impl,
    tag_classes = {"lock": _lock},
)
