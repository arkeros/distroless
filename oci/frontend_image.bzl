load("@rules_img//img:image.bzl", "image_index")
load("@tar.bzl", "mutate", "tar")
load("//oci/distroless:platforms.bzl", "ARCHITECTURE_PLATFORMS")
load("//oci/distroless/common:variables.bzl", "NONROOT")
load(":oci_image.bzl", "oci_image")
load("//oci/distroless/nginx:config.bzl", "NGINX_ARCHITECTURES")

def frontend_image_index(name, architectures):
    """frontend image index

    Args:
        name: base name of image
        architectures: all architectures included in index
    """
    image_index(
        name = name,
        manifests = [
            name + "_" + arch
            for arch in architectures
        ],
    )

def _statics_layer(name, srcs, owner, ownername, strip_prefix):
    tar(
        name = name + "_statics_layer",
        srcs = srcs,
        mutate = mutate(
            owner = owner,
            ownername = ownername,
            package_dir = "/var/www/html",
            strip_prefix = strip_prefix or native.package_name(),
        ),
    )
    return name + "_statics_layer"

def frontend_image(
        name,
        arch,
        distro = "debian13",
        srcs = None,
        statics_layer = None,
        base = None,
        owner = str(NONROOT),
        ownername = "nonroot",
        strip_prefix = None,
        **kwargs):
    """Build a single-arch frontend image serving static files with nginx.

    Provide either srcs (static files) or statics_layer (pre-built tar layer).

    Args:
        name: target name
        arch: target architecture (e.g., "amd64")
        distro: distribution to use (default: debian13)
        srcs: static files to serve
        statics_layer: pre-built tar layer (mutually exclusive with srcs)
        base: base image. Defaults to nginx mainline nonroot.
        owner: uid for static files (default: 65532/nonroot), only used with srcs
        ownername: uname for static files (default: nonroot), only used with srcs
        strip_prefix: prefix to strip from file paths, only used with srcs
        **kwargs: passed to oci_image (ignore_cves, env, etc.)
    """
    if distro not in NGINX_ARCHITECTURES:
        fail("unknown distro %r, expected one of: %s" % (distro, ", ".join(NGINX_ARCHITECTURES.keys())))
    if srcs and statics_layer:
        fail("srcs and statics_layer are mutually exclusive")
    if not srcs and not statics_layer:
        fail("one of srcs or statics_layer is required")

    if srcs:
        statics_layer = _statics_layer(name, srcs, owner, ownername, strip_prefix)

    oci_image(
        name = name + "_" + arch,
        base = base or "//oci/distroless/nginx:nginx_mainline_nonroot_" + arch + "_" + distro,
        layers = [statics_layer],
        platform = ARCHITECTURE_PLATFORMS[arch],
        **kwargs
    )

def frontend_images_all_arch(name, srcs, base = None, distro = "debian13", **kwargs):
    """Build frontend images for all architectures serving static files with nginx.

    Static files are placed in /var/www/html on top of the nginx base image.

    Args:
        name: target name
        srcs: static files to serve (e.g., a filegroup of built frontend assets)
        base: base image per arch, as a dict {"amd64": "//my:image_amd64", ...}.
            Defaults to nginx mainline nonroot.
        distro: distribution to use (default: debian13)
        **kwargs: passed to frontend_image (owner, ownername, strip_prefix, ignore_cves)
    """
    architectures = NGINX_ARCHITECTURES[distro]

    layer = _statics_layer(
        name,
        srcs,
        kwargs.pop("owner", str(NONROOT)),
        kwargs.pop("ownername", "nonroot"),
        kwargs.pop("strip_prefix", None),
    )

    [
        frontend_image(
            name = name,
            distro = distro,
            arch = arch,
            statics_layer = layer,
            base = base.get(arch) if base else None,
            **kwargs
        )
        for arch in architectures
    ]

    frontend_image_index(
        name = name,
        architectures = architectures,
    )
