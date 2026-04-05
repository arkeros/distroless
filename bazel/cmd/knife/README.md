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
knife apt versions oci/distroless/debian13.lock.json
```

Filter by architecture:

```bash
knife apt versions --arch amd64 oci/distroless/debian13.lock.json
```

### apt update

Update Debian snapshot timestamps in a manifest YAML file:

```bash
knife apt update oci/distroless/debian13.yaml
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

## Architecture

Commands use a noun-based package structure:

- `cmd/apt/` - `apt` noun (verbs: `update`, `versions`)
- `cmd/grype/` - `grype` noun (verbs: `update`)

Shared libraries:

- `bazel/pkg/grypedb` - grype database MODULE.bazel updater (via buildtools AST)
- `bazel/pkg/mod` - `bazel mod tidy` helper
- `oci/distroless/debian/lockfile` - apt lock file parsing
- `oci/distroless/debian/snapshot` - manifest parsing and snapshot fetching
