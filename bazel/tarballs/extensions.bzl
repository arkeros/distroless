"""Module extension `tarballs`: upstream prebuilt runtimes pinned by a lockfile.

One `lock(...)` tag reads a JSON lockfile written by `knife tarballs update`
and declares one repo per entry of every line's `archives`, named by the
entry's key, so image BUILD files reference e.g. `@nodejs_24_amd64`.
It also generates a hub repo (the tag's `name`) exporting `versions.bzl`
with the lines' versions, which the image's config.bzl loads to build the
SBOM identity (purl + CPE) without repeating the version by hand.

An entry is either an archive to unpack or a bare file to download. Every
entry sets exactly one of the two, and both shapes yield the same label
shape — the tag's `build_file_content` names what the image consumes, so
`@nodejs_24_amd64//:bin/node` and `@envoy_139_amd64//:envoy` read alike.

Lockfile schema (`<runtime>.lock.json`):

    {
      "schema_version": 1,
      "source": "nodejs" | "temurin" | "envoy",  # resolver knife refreshes it with
      "lines": [
        {
          "major": "24",
          "version": "24.19.0",
          "build": "8",                 # upstream build counter; absent for nodejs
          "archives": {
            # an archive: unpacked, `strip_prefix` dropped
            "<repo name>": {"url": "...", "sha256": "...", "strip_prefix": "..."},
            # or a bare file: downloaded to `file`, marked executable, not extracted
            "<repo name>": {"url": "...", "sha256": "...", "file": "envoy"}
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
            doc = "BUILD file content for every repo of this lockfile.",
        ),
    },
)

def _binary_repo_impl(rctx):
    # `executable`, because the whole point of a bare-file entry is an
    # upstream that publishes the program itself rather than an archive
    # holding it; downloads are 0644 otherwise and the tar rule that lays
    # it into a layer would carry that mode into the image.
    rctx.download(
        url = rctx.attr.url,
        output = rctx.attr.file,
        sha256 = rctx.attr.sha256,
        executable = True,
    )
    rctx.file("BUILD.bazel", rctx.attr.build_file_content)

_binary_repo = repository_rule(
    implementation = _binary_repo_impl,
    attrs = {
        "url": attr.string(mandatory = True),
        "sha256": attr.string(mandatory = True),
        "file": attr.string(mandatory = True),
        "build_file_content": attr.string(mandatory = True),
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
                    file = archive.get("file", "")
                    strip_prefix = archive.get("strip_prefix", "")
                    if (file == "") == (strip_prefix == ""):
                        fail("tarballs: {} entry {} must set exactly one of `file` and `strip_prefix`".format(lock.lockfile, repo_name))
                    if file:
                        _binary_repo(
                            name = repo_name,
                            url = archive["url"],
                            sha256 = archive["sha256"],
                            file = file,
                            build_file_content = lock.build_file_content,
                        )
                    else:
                        http_archive(
                            name = repo_name,
                            url = archive["url"],
                            sha256 = archive["sha256"],
                            strip_prefix = strip_prefix,
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

    # No root_module_direct_deps: the extension is used from more than one
    # MODULE include (js, java), each importing its own repos, and tidy
    # cannot tell which include a repo belongs to. Adding a line to a
    # lockfile means adding its repos to that include's use_repo by hand;
    # Bazel names the missing repo if it is forgotten.
    return mctx.extension_metadata(reproducible = True)

tarballs = module_extension(
    implementation = _tarballs_impl,
    tag_classes = {"lock": _lock},
)
