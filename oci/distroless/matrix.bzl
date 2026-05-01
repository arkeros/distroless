"Shared distroless OCI image matrix factory."

load("@rules_img//img:image.bzl", "image_index")
load("//oci/distroless:platforms.bzl", "ARCHITECTURE_PLATFORMS")
load("//oci/distroless/common:variables.bzl", "NONROOT")
load("//oci:oci_image.bzl", "oci_image")

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

def _resolve_image_ref(ref, context):
    if ref == None:
        return None

    suffix = _image_name("", context["mode"], context["user"], context["arch"], context["distro"])
    if ":" not in ref:
        return "{}:{}{}".format(ref, ref.rsplit("/", 1)[-1], suffix)
    return ref + suffix

def _resolve_layers(layers_fn, context):
    return layers_fn(struct(**context))

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
        base = None,
        entrypoint = None,
        env = None,
        annotations = None,
        index_annotations = None,
        debug_base = None,
        debug_layers = None,
        debug_entrypoint = None,
        debug_env = None,
        debug_annotations = None,
        debug_index_annotations = None,
        debug_ignore_cves = None,
        debug_vex = None,
        **kwargs):
    """Generates release/debug OCI images plus per-user manifest indexes.

    Args:
        name: image family name and target stem.
        distro: distro key such as "debian".
        architectures: architectures included in the matrix.
        layers: callback taking a context struct and returning release layers.
        base: optional base package or explicit target stem label.
        entrypoint: optional release entrypoint.
        env: optional release environment map.
        annotations: optional release image annotations.
        index_annotations: optional release index annotations.
        debug_base: optional debug base package or explicit target stem label.
        debug_layers: optional callback for debug layers. Defaults to release
            layers when a base image exists, or no extra layers for root images.
        debug_entrypoint: optional debug entrypoint override.
        debug_env: optional debug environment overlay.
        debug_annotations: optional debug image annotations override.
        debug_index_annotations: optional debug index annotations override.
        debug_ignore_cves: optional list extending kwargs["ignore_cves"] for
            debug images only — useful when debug layers add packages
            (e.g. busybox) not present in the release image.
        debug_vex: optional list of VEX document labels extending
            kwargs["vex"] for debug images only — useful when debug layers
            add packages whose CVEs need separate justifications. Mirrors
            debug_ignore_cves in shape.
        **kwargs: passed through to oci_image (e.g. ignore_cves, fail_on_severity).
    """

    release_env = env or {}
    release_annotations = annotations or {}
    release_index_annotations = index_annotations if index_annotations != None else release_annotations

    effective_debug_layers = debug_layers if debug_layers != None else (layers if base != None else None)
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
                base = _resolve_image_ref(base, release_context),
                layers = _resolve_layers(layers, release_context),
                entrypoint = entrypoint,
                env = release_env,
                annotations = release_annotations,
                **kwargs
            )

            debug_name = _image_name(name, "_debug", user, arch, distro)
            debug_context = _copy_dict(release_context)
            debug_context["mode"] = "_debug"
            debug_context["debug_name"] = debug_name

            resolved_debug_base = _resolve_image_ref(debug_base, debug_context)
            if resolved_debug_base == None:
                resolved_debug_base = _resolve_image_ref(base, debug_context) if base != None else ":" + release_name

            resolved_debug_layers = _resolve_layers(effective_debug_layers, debug_context) if effective_debug_layers != None else []

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
                base = resolved_debug_base,
                layers = resolved_debug_layers,
                entrypoint = effective_debug_entrypoint,
                env = effective_debug_env,
                annotations = effective_debug_annotations,
                **debug_kwargs
            )

    for (mode, mode_annotations) in [
        ("", release_index_annotations),
        ("_debug", effective_debug_index_annotations),
    ]:
        for (user, _, _) in USER_VARIANTS:
            image_index(
                name = _index_name(name, mode, user, distro),
                annotations = mode_annotations,
                manifests = [
                    _image_name(name, mode, user, arch, distro)
                    for arch in architectures
                ],
            )
