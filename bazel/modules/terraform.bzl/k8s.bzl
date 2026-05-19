"""Re-export shim for `load("@terraform.bzl//:k8s.bzl", ...)`.

Kubernetes resource constructors. Implementations live in
`terraform/resources/k8s.bzl`; see `tf_root` for how the returned
structs flow into `main.tf.json`.
"""

load(
    "//terraform/resources:k8s.bzl",
    _kubernetes_job_v1 = "kubernetes_job_v1",
    _kubernetes_manifest = "kubernetes_manifest",
    _kubernetes_provider = "kubernetes_provider",
    _kubernetes_secret_v1 = "kubernetes_secret_v1",
)

kubernetes_provider = _kubernetes_provider
kubernetes_manifest = _kubernetes_manifest
kubernetes_job_v1 = _kubernetes_job_v1
kubernetes_secret_v1 = _kubernetes_secret_v1
