"""Bifrost Bazel macro for generating platform manifests from Starlark."""

load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("//devtools/bifrost:render.bzl", "bifrost_render")

def bifrost_service(
        name,
        port,
        gcp,
        resources,
        image = None,
        image_push = None,
        args = None,
        service_account_name = None,
        secret_files = None,
        probes = None,
        autoscaling = None,
        kubernetes = None,
        checked_in = None,
        targets = None,
        visibility = None):
    """Generate platform manifests from a Bifrost service spec defined in Starlark.

    This macro generates a Bifrost service JSON from Starlark parameters and
    runs `bifrost render` to produce Cloud Run, Kubernetes, and Terraform outputs.

    Example:
        bifrost_service(
            name = "registry",
            image = "registry",
            args = ["--upstream=ghcr.io"],
            port = 8080,
            resources = {
                "requests": {"cpu": "250m", "memory": "256Mi"},
                "limits": {"cpu": "1000m", "memory": "256Mi"},
            },
            gcp = {
                "projectId": "senku-prod",
                "region": "europe-west3",
            },
        )

    This produces targets:
        :<name>.service.json    — generated Bifrost input spec
        :<name>.cloudrun.yaml   — Cloud Run (Knative) manifest
        :<name>.k8s.yaml        — Kubernetes manifests (SA, Deployment, HPA, Service)
        :<name>.terraform.tf    — Terraform HCL for runtime identity

    Args:
        name: Service name (used in metadata.name and as target prefix).
        image: Plain string container image reference (e.g. "registry"). Mutually
            exclusive with image_push.
        image_push: Bazel label pointing to an image_push target (e.g.
            "//oci/cmd/registry:image_nonroot_push"). At build time, the deploy
            manifest is read to resolve a digest-pinned image reference. Mutually
            exclusive with image.
        port: Container port number.
        gcp: GCP configuration dict. Must include "projectId", "projectNumber", and
            "region". Example:
            {"projectId": "my-proj", "projectNumber": "123456789012", "region": "us-central1"}.
        resources: Resource requirements dict with "requests" and/or "limits" sub-dicts,
            each containing "cpu" and "memory". Example: {"limits": {"cpu": "1000m", "memory": "256Mi"}}.
        args: Optional list of container arguments.
        service_account_name: Optional GSA email. Auto-generated from name + projectId if omitted.
        secret_files: Optional list of secret file dicts. Each dict has "secret" (name or
            projects/P/secrets/N path) and "path" (mount path). Example:
            [{"secret": "my-secret", "path": "/run/secrets/env.json"}].
        probes: Optional probe paths dict. Example: {"startupPath": "/healthz", "livenessPath": "/healthz"}.
        autoscaling: Optional autoscaling dict. Example: {"min": 0, "max": 5, "concurrency": 100}.
        kubernetes: Optional Kubernetes config dict. Example: {"namespace": "prod", "serviceType": "LoadBalancer"}.
        checked_in: Optional dict mapping render targets to checked-in output paths.
            For each mapped target, the macro creates a `write_source_file` update
            target named `:<name>_<target>_update` and adds a generated-file header
            pointing to that update command.
        targets: List of render targets to generate. Defaults to ["cloudrun", "k8s", "terraform"].
            Use this to generate only a subset of outputs.
        visibility: Bazel visibility for all generated targets.
    """
    if image and image_push:
        fail("Cannot specify both 'image' and 'image_push'")
    if not image and not image_push:
        fail("Must specify either 'image' or 'image_push'")

    if targets == None:
        targets = ["cloudrun", "k8s", "terraform"]
    if checked_in == None:
        checked_in = {}

    # Build the service spec dict
    spec = {}
    spec["image"] = image or name
    if service_account_name:
        spec["serviceAccountName"] = service_account_name
    if args:
        spec["args"] = args
    spec["port"] = port
    spec["resources"] = resources
    if secret_files:
        spec["secretFiles"] = secret_files
    if probes:
        spec["probes"] = probes
    if autoscaling:
        spec["autoscaling"] = autoscaling
    spec["gcp"] = gcp
    if kubernetes != None:
        spec["kubernetes"] = kubernetes

    service_obj = {}
    service_obj["apiVersion"] = "bifrost.apotema.cloud/v1alpha1"
    service_obj["kind"] = "Service"
    service_obj["metadata"] = {"name": name}
    service_obj["spec"] = spec

    service_json = json.encode_indent(service_obj, indent = "  ") + "\n"

    json_target = name + ".service.json"
    write_file(
        name = name + "_service_json",
        out = json_target,
        content = [service_json],
        visibility = visibility,
    )

    # Generate each render target
    for target in targets:
        if target == "terraform":
            out_file = name + ".terraform.tf"
        else:
            out_file = name + "." + target + ".yaml"

        header = ""
        if target in checked_in:
            update_target = name + "_" + target + "_update"
            pkg = native.package_name()
            update_label = "//:%s" % update_target if pkg == "" else "//%s:%s" % (pkg, update_target)
            header = "\n".join([
                "# Generated by bifrost.bzl",
                "# To update this file, run:",
                "#   bazel run %s" % update_label,
                "",
            ])

        bifrost_render(
            name = name + "_" + target,
            spec = ":" + name + "_service_json",
            target = target,
            image_push = image_push,
            header = header if header else None,
            out = out_file,
            visibility = visibility,
        )

        if target in checked_in:
            write_source_file(
                name = name + "_" + target + "_update",
                in_file = ":" + name + "_" + target,
                out_file = checked_in[target],
            )
