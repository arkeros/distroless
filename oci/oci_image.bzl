load("@rules_img//img:image.bzl", "image_manifest")
load("@rules_img//img:load.bzl", "image_load")
load("@rules_img//img:push.bzl", "image_push")
load("@tar.bzl", "tar")
load(":supply_chain.bzl", "image_supply_chain")

def oci_image(
        name,
        fail_on_severity = "high",
        ignore_cves = None,
        vex = None,
        **kwargs):
    """Build an OCI container image with SBOM + CVE scanning.

    Wraps rules_img's `image_manifest` with `image_load`, sidecar tarballs,
    and `image_supply_chain` (SBOM + grype scan + policy test). Push targets
    are created by kustomize() based on deployment context.

    Args:
        name: Target name.
        fail_on_severity: CVE severity threshold for the policy test.
            Default "high".
        ignore_cves: List of CVE IDs to allow-list in the policy test. Prefer
            `vex` for anything with a defensible justification; reserve this
            for unjustifiable noise.
        vex: List of OpenVEX document labels (see //oci:vex.bzl). Statements
            with status=not_affected or fixed remove matching results from
            the grype scan before the policy test evaluates severity.
        **kwargs: Passed to image_manifest (base, layers, entrypoint, env, etc.).
    """
    image_manifest(
        name = name,
        **kwargs
    )

    image_supply_chain(
        fail_on_severity = fail_on_severity,
        ignore_cves = ignore_cves,
        vex = vex,
        image = ":" + name,
    )

    image_load(
        name = name + "_load",
        image = ":%s" % name,
        tag_list = [
            "bazel/%s:%s" % (native.package_name(), name.replace("_", "-")),
        ],
    )

    native.filegroup(
        name = name + ".tar",
        srcs = [
            ":%s_load" % name,
        ],
        output_group = "tarball",
        tags = ["manual"],
    )

    native.filegroup(
        name = name + ".oci_layout",
        srcs = [
            ":%s" % name,
        ],
        output_group = "oci_layout",
    )

def manifest_image(
        name,
        manifest,
        registry = None,
        repository_prefix = None,
        visibility = None):
    """Packages a Kubernetes manifest into an OCI image.

    Creates a container image containing the given manifest file,
    suitable for use with GitOps tools that pull manifests from registries.

    Args:
        name: The name of the target.
        manifest: The manifest target to package.
        registry: Registry URL for pushing (e.g., "gcr.io", "index.docker.io").
        repository_prefix: Repository prefix; package name is appended automatically.
        visibility: The visibility of the target.
    """
    tar(
        name = "{}_layer".format(manifest),
        srcs = [":{}".format(manifest)],
        compress = "gzip",
    )

    oci_image(
        name = name,
        layers = [":{}_layer".format(manifest)],
        visibility = visibility,
        platform = "//bazel/platforms:linux_amd64",
    )

    if registry and repository_prefix:
        image_push(
            name = name + "_push",
            image = ":" + name,
            registry = registry,
            repository = repository_prefix + "/" + native.package_name(),
            visibility = visibility,
        )
