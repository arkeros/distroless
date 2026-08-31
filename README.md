My public monorepo

## Setup

Install Bazelisk and direnv first.

On macOS with Homebrew:

```bash
brew install bazelisk direnv
```

If you are not on macOS, use the [official direnv installation guide](https://direnv.net/docs/installation.html). Then [hook direnv into your shell](https://direnv.net/docs/hook.html). Restart your shell after that.

This repo uses [.envrc](./.envrc) to set up the Bazel-backed local development environment.

Before enabling `direnv`, generate the Bazel-backed tool shims:

```bash
bazel run //tools:dev
direnv allow
```

That authorizes this repo's `.envrc` so `direnv` can load it automatically when you enter the workspace.

In this repo, `//tools:dev` already exposes commands through `lazy_bazel_env`. That includes [knife](./bazel/cmd/knife), so after `direnv allow` you can run `knife ...` directly from the repo root without adding your own shell function.
