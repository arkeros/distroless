# distroless

Minimal container base images, built from source with Bazel. Every published
image carries a signature and a CycloneDX SBOM signed by this repository's
workflow, and SLSA Build L3 provenance written by
[slsa-github-generator](https://github.com/slsa-framework/slsa-github-generator)
— a build platform component our own build cannot forge. All three are bound
to the image digest. The L3 claim is self-assessed against the SLSA
requirements; there is no third-party assessment of the build platform.

An image holds your application and its runtime dependencies and nothing else —
no shell, no package manager, no init system. The `_debug` variants add busybox
for the times you need to look inside one.

## Pull

```bash
docker pull distroless.io/<image>:<tag>
```

`distroless.io` is a pull-through proxy in front of the canonical bytes on
GHCR — see [`//oci/cmd/registry`](./oci/cmd/registry/README.md). Both surfaces
serve the same digests and verify against the same policy.

## Verify

Nothing here is trustworthy because this README says so. The verification
policy — OIDC issuer and workflow identity — is the single source of truth in
[`//oci:cosign_policy.bzl`](./oci/cosign_policy.bzl). Pin both flags, so a
signature minted from a different repository or workflow is rejected:

```bash
cosign verify \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --certificate-identity-regexp='...'  \
    distroless.io/<image>:<tag>
```

The exact regexp, the SBOM attestation, and the `slsa-verifier` command for
provenance — which is signed by the generator, not by us, and is bound to this
repository by its `--source-uri` — are in [the image reference](./docs/images.md).

## Images

Publicly mirrored:

| Image | Contents |
| --- | --- |
| `bash` | A shell, for entrypoint scripts and build steps |
| `java` | JDK 17, 21 and 25 |
| `node` | Node 20, 24 and 26 |
| `nginx` | nginx stable and mainline, serving a webroot as nonroot |

Built here as bases for the above, not separately mirrored: `static`, `cc`,
`python`.

Images come in a matrix of **distro** (`debian`, `hummingbird`), **user**
(`root`, `nonroot`) and **debug** (with or without busybox), for `amd64` and
`arm64`. Not every family publishes every cell — see
[the image reference](./docs/images.md) for the tags each one actually
ships.

## How it is built

Packages are resolved ahead of time into checked-in lockfiles
(`debian.lock.json`, `hummingbird.lock.json`, the nginx locks) and assembled
into layers by Bazel. Nothing runs `apt-get` at image build time, so building
an old commit today gives you the image that commit described.

Every image is scanned before it can be published, and the scan is gated three
ways: on findings, on suppressions that have stopped matching anything, and on
components no scanner could have matched in the first place. That last gate is
the interesting one — a clean report means nothing if the scanner had no way to
route what it was looking at. See
[`//oci:supply_chain.bzl`](./oci/supply_chain.bzl) and
[ADR 0007](./docs/adr/0007-hummingbird-rpm-base.md).

The vocabulary these docs use is fixed in [CONTEXT.md](./CONTEXT.md), and the
decisions behind it live in [docs/adr/](./docs/adr/).

## Development

Install Bazelisk and direnv. On macOS with Homebrew:

```bash
brew install bazelisk direnv
```

Otherwise follow the [direnv installation guide](https://direnv.net/docs/installation.html)
and [hook it into your shell](https://direnv.net/docs/hook.html), then restart
your shell.

Generate the Bazel-backed tool shims, then authorize [`.envrc`](./.envrc) so
direnv loads them whenever you enter the workspace:

```bash
bazel run //tools:dev
direnv allow
```

`//tools:dev` exposes this repo's tools through `lazy_bazel_env`, so `knife`,
`crane`, `cosign` and the rest are on `PATH` without installing anything
yourself.

Build and test:

```bash
bazel build //images/... //oci/...
bazel test //images/... //oci/...
```

Package sets are regenerated with [`knife`](./bazel/cmd/knife), not by hand:

```bash
knife apt update     # Debian package lockfiles
knife grype update   # vulnerability database pin
```

## Infrastructure

[`//infra`](./infra/README.md) is plain Terraform for the durable pieces — the
Artifact Registry repository deploys pull from, the registry service's runtime
identity, and the GitHub OIDC federation CI authenticates with. It is applied
by hand and deliberately kept out of the deploy path.

## License

[Apache 2.0](./LICENSE).
