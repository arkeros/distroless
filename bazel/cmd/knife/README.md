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

### deb versions

Display package versions from a Debian lock file:

```bash
knife debian versions oci/distroless/debian13.lock.json
```

Filter by architecture:

```bash
knife debian versions --arch amd64 oci/distroless/debian13.lock.json
```

### grype-db update

Update the grype vulnerability database to the latest version:

```bash
knife grype update
```

This command:

1. Fetches the latest database metadata from grype.anchore.io
2. Updates `bazel/include/oci.MODULE.bazel` with the new URL and SHA256
3. Runs `bazel mod tidy` to update the lockfile

### snapshots update

Update Debian snapshot timestamps in a manifest YAML file:

```bash
knife snapshots update oci/distroless/debian13.yaml
```

This command:

1. Fetches the latest snapshot timestamps from snapshot.debian.org
2. Updates all source URLs in the YAML file with the new timestamps
3. Prints a reminder to regenerate the lockfile

## Architecture

Commands use a noun-based package structure:

- `cmd/debian/` - `debian` noun (verbs: `versions`)
- `cmd/grype/` - `grype` noun (verbs: `update`)
- `cmd/snapshots/` - `snapshots` noun (verbs: `update`)

Shared libraries:

- `bazel/pkg/grypedb` - grype database MODULE.bazel updater (via buildtools AST)
- `bazel/pkg/mod` - `bazel mod tidy` helper
- `oci/distroless/debian/lockfile` - Debian lock file parsing
- `oci/distroless/debian/snapshot` - manifest parsing and snapshot fetching
