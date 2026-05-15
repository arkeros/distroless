"""JSON lockfile schema + parsing helpers.

Schema (`<name>_install.json`):

    {
      "schema_version": 1,
      "repo": {
        "url": "https://.../public-hummingbird",
        "revision": "1778835791",          # repomd.xml <revision>; cache-friendly anchor
        "repomd_sha256": "..."             # repomd.xml digest at lock time
      },
      "packages": {
        "<name>": {
          "<arch>": {                       # "noarch" | "x86_64" | "aarch64" | ...
            "version": "2.42-13.hum1",
            "sha256": "...",
            "path": "Packages/g/glibc-2.42-13.hum1.x86_64.rpm",
            "size": 1234567
          }
        }
      }
    }

`packages[name][arch]` is the cross-product surface; `noarch` packages have
exactly one nested key, arch-specific packages have one per declared
architecture. The pin tool errors if the cross-product is incomplete.
"""

def parse_lockfile(mctx, label):
    """Reads `label` as JSON and returns the decoded dict.

    Module-extension contexts can read input files via `mctx.read(label)`.
    """
    content = mctx.read(label)
    parsed = json.decode(content)
    if parsed.get("schema_version") != 1:
        fail("rules_rpm: unsupported lockfile schema_version: %s" % parsed.get("schema_version"))
    return parsed
