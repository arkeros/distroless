load("@tar.bzl", "tar")
load("@grype.bzl", "cve_policy")
load("@rules_img//img:image.bzl", "image_manifest")
load("@rules_img//img:load.bzl", "image_load")
load("@rules_img//img:push.bzl", "image_push")

def oci_image(
        name,
        fail_on_severity = "high",
        ignore_cves = None,
        **kwargs):
    """Build an OCI container image with CVE scanning.

    This macro wraps rules_img's image_manifest with load and CVE scanning.
    Push targets are created by kustomize() based on deployment context.

    CVE scanning is handled via aspects (syft_sbom_aspect + grype_aspect)
    attached globally in .bazelrc. Per-target CVE policy can be configured
    via fail_on_severity and ignore_cves parameters.

    Args:
        name: Target name
        fail_on_severity: CVE severity threshold (default: "high")
        ignore_cves: List of CVE IDs to ignore in scanning
        **kwargs: Passed to image_manifest (base, layers, entrypoint, env, etc.)
    """

    # Wire CVE policy as aspect_hint so grype_aspect can read it
    aspect_hints = kwargs.pop("aspect_hints", [])
    if ignore_cves or fail_on_severity != "high":
        cve_policy(
            name = name + "_cve_policy",
            fail_on_severity = fail_on_severity,
            ignore_cves = ignore_cves or [],
        )
        aspect_hints = aspect_hints + [":%s_cve_policy" % name]

    image_manifest(
        name = name,
        aspect_hints = aspect_hints,
        **kwargs
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
