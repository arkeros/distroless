"Shared distroless OCI image matrix factory."

load("@rules_img//img:image.bzl", "image_index")
load("//oci/distroless:platforms.bzl", "ARCHITECTURE_PLATFORMS")
load("//oci/distroless/common:variables.bzl", "NONROOT")
load("//oci:oci_image.bzl", "oci_image")
load("//oci:supply_chain.bzl", "image_sbom")

USER_VARIANTS = [
    ("root", 0, "/"),
    ("nonroot", NONROOT, "/home/nonroot"),
]

def _merge_dicts(base, overlay):
    merged = {}
    for source in [base, overlay]:
        if source:
            for key, value in source.items():
                merged[key] = value
    return merged

def _copy_dict(source):
    copied = {}
    for key, value in source.items():
        copied[key] = value
    return copied

def _image_name(name, mode, user, arch, distro):
    return "{}{}_{}_{}_{}".format(name, mode, user, arch, distro)

def _index_name(name, mode, user, distro):
    return "{}{}_{}_{}".format(name, mode, user, distro)

# Parent dirs companion to per-package dpkg_statusd outputs from
# //oci/distroless/common:package.BUILD.tmpl. See
# //oci/distroless/common:dpkg_status_d_dirs for rationale.
_DPKG_STATUS_D_DIRS = "//oci/distroless/common:dpkg_status_d_dirs"

def _resolve_layers(layers_fn, context):
    return [_DPKG_STATUS_D_DIRS] + layers_fn(struct(**context))

def _emit_image(arch, uid, working_dir, **kwargs):
    kwargs["platform"] = ARCHITECTURE_PLATFORMS[arch]
    kwargs["user"] = "%d" % uid
    kwargs["working_dir"] = working_dir
    oci_image(**kwargs)

def distroless_matrix(
        name,
        distro,
        architectures,
        layers,
        entrypoint = None,
        env = None,
        annotations = None,
        index_annotations = None,
        created = None,
        debug_layers = None,
        debug_entrypoint = None,
        debug_env = None,
        debug_annotations = None,
        debug_index_annotations = None,
        debug_ignore_cves = None,
        debug_vex = None,
        **kwargs):
    """Generates release/debug OCI images plus per-user manifest indexes.

    Each image is composed from an explicit list of layers — no `base =`
    inheritance. The `layers` callback returns the full layer composition
    (rootfs + packages + rpmdb), and `debug_layers` does the same for the
    `_debug` variant (typically adds busybox + a busybox-aware rpmdb).
    See //oci/distroless/cc/config.bzl:cc_layers for the canonical pattern.

    Args:
        name: image family name and target stem.
        distro: distro key such as "debian".
        architectures: architectures included in the matrix.
        layers: callback `(ctx) -> [Label]` returning release layers.
            ctx exposes name/mode/user/uid/arch/distro/working_dir.
        entrypoint: optional release entrypoint.
        env: optional release environment map.
        annotations: optional release image annotations.
        index_annotations: optional release index annotations.
        debug_layers: callback for debug layers. Required if the debug
            variant should differ from release (e.g. add busybox). When
            None, debug_image == release_image (same layers, same env).
        debug_entrypoint: optional debug entrypoint override.
        debug_env: optional debug environment overlay (merged onto env).
        debug_annotations: optional debug image annotations override.
        debug_index_annotations: optional debug index annotations override.
        debug_ignore_cves: optional list extending kwargs["ignore_cves"] for
            debug images only — useful when debug layers add packages
            (e.g. busybox) not present in the release image.
        debug_vex: optional list of VEX document labels extending
            kwargs["vex"] for debug images only — for justifications that
            apply only to packages added by debug layers. Mirrors
            debug_ignore_cves in shape.
        created: optional label of a one-file RFC 3339 timestamp,
            forwarded to `oci_image(created = ...)`. Shared across
            release and debug variants — same upstream-snapshot anchor
            applies. See //oci:created_timestamp.bzl.
        **kwargs: passed through to oci_image (e.g. ignore_cves, fail_on_severity).
    """

    release_env = env or {}
    release_annotations = annotations or {}
    release_index_annotations = index_annotations if index_annotations != None else release_annotations

    effective_debug_layers = debug_layers if debug_layers != None else layers
    effective_debug_entrypoint = debug_entrypoint if debug_entrypoint != None else entrypoint
    effective_debug_env = _merge_dicts(release_env, debug_env or {})
    effective_debug_annotations = debug_annotations if debug_annotations != None else release_annotations
    effective_debug_index_annotations = debug_index_annotations if debug_index_annotations != None else release_index_annotations

    for arch in architectures:
        for (user, uid, working_dir) in USER_VARIANTS:
            release_name = _image_name(name, "", user, arch, distro)
            release_context = {
                "name": name,
                "mode": "",
                "user": user,
                "uid": "%d" % uid,
                "arch": arch,
                "distro": distro,
                "working_dir": working_dir,
                "release_name": release_name,
            }

            _emit_image(
                name = release_name,
                arch = arch,
                uid = uid,
                working_dir = working_dir,
                layers = _resolve_layers(layers, release_context),
                entrypoint = entrypoint,
                env = release_env,
                annotations = release_annotations,
                created = created,
                **kwargs
            )

            debug_name = _image_name(name, "_debug", user, arch, distro)
            debug_context = _copy_dict(release_context)
            debug_context["mode"] = "_debug"
            debug_context["debug_name"] = debug_name

            debug_kwargs = _copy_dict(kwargs)
            if debug_ignore_cves:
                debug_kwargs["ignore_cves"] = (debug_kwargs.get("ignore_cves") or []) + debug_ignore_cves
            if debug_vex:
                debug_kwargs["vex"] = (debug_kwargs.get("vex") or []) + debug_vex

            _emit_image(
                name = debug_name,
                arch = arch,
                uid = uid,
                working_dir = working_dir,
                layers = _resolve_layers(effective_debug_layers, debug_context),
                entrypoint = effective_debug_entrypoint,
                env = effective_debug_env,
                annotations = effective_debug_annotations,
                created = created,
                **debug_kwargs
            )

    for (mode, mode_annotations) in [
        ("", release_index_annotations),
        ("_debug", effective_debug_index_annotations),
    ]:
        for (user, _, _) in USER_VARIANTS:
            index_name = _index_name(name, mode, user, distro)
            image_index(
                name = index_name,
                annotations = mode_annotations,
                manifests = [
                    _image_name(name, mode, user, arch, distro)
                    for arch in architectures
                ],
            )

            # Index-level CycloneDX SBOM, used as the predicate for mirror_push's
            # SBOM attestation. Per-arch CVE testing already runs via oci_image;
            # this is just the unified-across-archs materials manifest.
            image_sbom(image = ":" + index_name)
