load("@rules_img//img:image.bzl", "image_index")
load("@tar.bzl", "mutate", "tar")
load("//oci/distroless:platforms.bzl", "ARCHITECTURE_PLATFORMS")
load("//oci/distroless/common:variables.bzl", "NONROOT")
load(":oci_image.bzl", "oci_image")
load("//oci/distroless/nginx:config.bzl", "NGINX_ARCHITECTURES")

NGINX_FRONTEND_DEFAULT_CHANNEL = "stable"

# Canonical on-image location and owner for the statics nginx serves. The
# web root matches the nginx base's `root` directive
# (see //oci/distroless/nginx:default.conf); the username matches the UID
# in //oci/distroless/common:variables.bzl#USER_IDS. Exposed so callers
# producing their own statics_layer (e.g. react_static_layer) can line up
# on the exact same paths/owner without hardcoding them independently.
NGINX_WEB_ROOT = "/var/www/html"
NGINX_USERNAME = "nonroot"
NGINX_UID = NONROOT

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
            package_dir = NGINX_WEB_ROOT,
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
        statics_layer: pre-built tar layer label, or list of labels composed
            as multiple OCI layers (py_image_layer style). List form is
            useful when logical groups (webroot, nginx conf) want
            independent layer-cache boundaries. Mutually exclusive with srcs.
        base: base image. Defaults to the nginx stable nonroot image.
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

    # Accept either a single label (historical shape) or a list of labels
    # (py_image_layer pattern). Callers like react_static_layer emit
    # multiple logically-separated tars (webroot, nginx conf) and pass
    # them together so each keeps its own layer-cache boundary.
    layers = statics_layer if type(statics_layer) == "list" else [statics_layer]

    oci_image(
        name = name + "_" + arch,
        # Wrap in Label() so the default resolves to @senku regardless of the
        # caller's repo — same cross-repo pattern as go_image.bzl's base.
        base = base or Label("//oci/distroless/nginx:nginx_%s_nonroot_%s_%s" % (NGINX_FRONTEND_DEFAULT_CHANNEL, arch, distro)),
        layers = layers,
        platform = ARCHITECTURE_PLATFORMS[arch],
        **kwargs
    )

def frontend_images_all_arch(name, srcs = None, statics_layer = None, base = None, distro = "debian13", **kwargs):
    """Build frontend images for all architectures serving static files with nginx.

    Static files are placed in /var/www/html on top of the nginx base image.
    Provide either srcs (static files; a layer is built for you) or
    statics_layer (a pre-built tar layer with final on-disk paths).

    Args:
        name: target name
        srcs: static files to serve (e.g., a filegroup of built frontend assets).
            Mutually exclusive with statics_layer.
        statics_layer: pre-built tar layer label, or list of labels composed
            as multiple OCI layers. Use this when the layer cannot be
            produced from a flat srcs + strip_prefix — e.g.
            react_static_layer, which emits one tar per logical group
            (webroot, nginx conf) for independent cache boundaries.
            Mutually exclusive with srcs.
        base: base image per arch, as a dict {"amd64": "//my:image_amd64", ...}.
            Defaults to the nginx stable nonroot image.
        distro: distribution to use (default: debian13)
        **kwargs: passed to frontend_image (owner, ownername, strip_prefix, ignore_cves)
    """
    if srcs and statics_layer:
        fail("srcs and statics_layer are mutually exclusive")
    if not srcs and not statics_layer:
        fail("one of srcs or statics_layer is required")

    architectures = NGINX_ARCHITECTURES[distro]

    if srcs:
        layer = _statics_layer(
            name,
            srcs,
            kwargs.pop("owner", str(NONROOT)),
            kwargs.pop("ownername", "nonroot"),
            kwargs.pop("strip_prefix", None),
        )
    else:
        layer = statics_layer

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
