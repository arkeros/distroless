# Distroless Nginx

Minimal nginx container images based on Debian Trixie (13), built from official [nginx.org](https://nginx.org/en/linux_packages.html#Debian) packages.

Images are published to [GitHub Container Registry](https://github.com/arkeros/senku/pkgs/container/senku%2Fnginx/).

```bash
docker pull ghcr.io/arkeros/senku/nginx:latest
```

## Channels

| Channel  | Version | Tags                         |
|----------|---------|------------------------------|
| Mainline | 1.29.x  | `latest`, `mainline`, `1.29` |
| Stable   | 1.28.x  | `stable`, `1.28`             |

## Usage

```bash
# Load into Docker
bazel run //distroless/nginx:nginx_mainline_nonroot_arm64_debian13_load

# Run
docker run --rm -p 8080:8080 bazel/distroless/nginx:nginx-mainline-nonroot-arm64-debian13
```

The default configuration listens on port **8080** and serves from `/var/www/html`.

## Configuration

The image ships with a nonroot-friendly nginx config:

- **`/etc/nginx/nginx.conf`** - Main config (pid/temp in `/tmp`, logs to stdout/stderr, `server_tokens off`)
- **`/etc/nginx/conf.d/default.conf`** - Default server block (port 8080, SPA-friendly `try_files`)

Override by mounting your own config:

```bash
docker run --rm -p 8080:8080 \
  -v ./my-site:/var/www/html:ro \
  -v ./my.conf:/etc/nginx/conf.d/default.conf:ro \
  bazel/distroless/nginx:nginx-mainline-nonroot-arm64-debian13
```

## Frontend Images

Use the macros in `frontend.bzl` to build images for static frontends (SPAs, static sites).
Static files are placed in `/var/www/html` on top of the nginx mainline nonroot base.

### Multi-arch (recommended)

```starlark
load("//distroless/nginx:frontend.bzl", "frontend_images_all_arch")

frontend_images_all_arch(
    name = "my_app",
    srcs = [":build"],
)
```

Creates `my_app_amd64`, `my_app_arm64`, and `my_app` (multi-arch index).

### Single-arch

```starlark
load("//distroless/nginx:frontend.bzl", "frontend_image")

frontend_image(
    name = "my_app",
    srcs = [":build"],
    arch = "amd64",
)
```

Creates `my_app_amd64`.

### Options

| Parameter       | Default           | Description                              |
|-----------------|-------------------|------------------------------------------|
| `srcs`          | —                 | Static files to serve                    |
| `statics_layer` | —                 | Pre-built tar layer (alternative to srcs)|
| `base`          | mainline nonroot  | Custom base image (dict for all_arch)    |
| `owner`         | `"65532"`         | UID for static files                     |
| `ownername`     | `"nonroot"`       | Username for static files                |
| `strip_prefix`  | package name      | Prefix to strip from file paths          |
| `**kwargs`      | —                 | Passed to `oci_image` (e.g., `ignore_cves`, `env`) |

## Variants

Each channel produces images for:

- **Users**: `root`, `nonroot` (UID 65532)
- **Architectures**: `amd64`, `arm64`
- **Debug**: includes busybox shell

Target naming: `nginx_{mainline,stable}[_debug]_{root,nonroot}_{amd64,arm64}_debian13`

## Updating

The nginx.org repo is not a Debian snapshot, so the lock file resolver picks the oldest version. After regenerating lock files, manually update the nginx entries to the latest version:

```bash
# Regenerate (will resolve to oldest version)
bazel run @nginx_stable//:lock
bazel run @nginx_mainline//:lock

# Then update the nginx package entries in the lock files
# to point to the latest version with correct SHA256
```
