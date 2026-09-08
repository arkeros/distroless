# knife - Swiss-army knife for Bazel build management

A command-line tool for managing Bazel build infrastructure tasks.

The tool uses the familiar `<context> <noun> <verb>` style of CLI interactions. For example, to update the grype database, you would run:

```bash
knife grype update
```

## Setup

See the repo [Setup section](../../../README.md) for Bazelisk installation, `direnv`, `bazel run //tools:dev`, and `direnv allow`.

After that, `knife` is available from the repo root.

## Usage

### apt versions

Display package versions from an apt lock file:

```bash
knife apt versions images/debian.lock.json
```

Filter by architecture:

```bash
knife apt versions --arch amd64 images/debian.lock.json
```

### apt update

Update Debian snapshot timestamps in a manifest YAML file:

```bash
knife apt update images/debian.yaml
```

This command:

1. Fetches the latest snapshot timestamps from snapshot.debian.org
2. Updates all source URLs in the YAML file with the new timestamps
3. Prints a reminder to regenerate the lockfile

### grype update

Update the grype vulnerability database to the latest version:

```bash
knife grype update
```

This command:

1. Fetches the latest database metadata from grype.anchore.io
2. Updates `bazel/include/oci.MODULE.bazel` with the new URL and SHA256
3. Runs `bazel mod tidy` to update the lockfile

### tarballs update

Move every release line in an upstream tarball lockfile to its newest release:

```bash
knife tarballs update images/nodejs/nodejs.lock.json
knife tarballs update images/java/temurin.lock.json
knife tarballs update images/postgres/walg.lock.json
```

This command:

1. Asks the lockfile's upstream (nodejs.org, the Adoptium API or the wal-g releases API) for the newest release of every major line the file lists
2. Rewrites the lockfile with the new versions, URLs and checksums
3. Runs `bazel mod tidy` to update the lockfile

Which majors a lockfile lists is a policy decision made by hand in that file; the command never adds or drops a line. For Temurin, a line only moves once the JRE and JDK for both architectures are published at the same release. For wal-g, drafts and prereleases are skipped and the highest version on the line wins, not the most recently created release.

## Architecture

Commands use a noun-based package structure:

- `cmd/apt/` - `apt` noun (verbs: `update`, `versions`)
- `cmd/grype/` - `grype` noun (verbs: `update`)
- `cmd/tarballs/` - `tarballs` noun (verbs: `update`)

Shared libraries:

- `bazel/grypedb` - grype database MODULE.bazel updater (via buildtools AST)
- `bazel/mod` - `bazel mod tidy` helper
- `bazel/tarballs` - upstream tarball lockfile and its nodejs.org / Adoptium resolvers; `extensions.bzl` there is the module extension that consumes the lockfile
- `oci/debian/lockfile` - apt lock file parsing
- `oci/debian/snapshot` - manifest parsing and snapshot fetching
