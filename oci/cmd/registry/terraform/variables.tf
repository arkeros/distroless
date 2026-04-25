variable "project_id" {
  type        = string
  description = "GCP project hosting every region copy of the registry service."
}

variable "image" {
  type        = string
  description = "Fully-qualified, digest-pinned container image URI for the registry proxy. Populated by the Bazel-generated `image.auto.tfvars.json` (`bazel run //oci/cmd/registry:image_tfvars`). The rule emits the full `<registry>/<repo>@sha256:...` URI, not just the digest — so swapping registries later is a change in the `image_push` target, not in this root."
  validation {
    condition     = can(regex("@sha256:[0-9a-f]{64}$", var.image))
    error_message = "image must be a digest-pinned URI: `<registry>/<repo>@sha256:<64-hex>`."
  }
}

variable "regions" {
  type        = set(string)
  description = "GCP regions to deploy the registry to. One Cloud Run service per region — all share the same name (`registry`) and runtime SA, fronted by the LB stack in `infra/cloud/gcp/lb`."
}

variable "upstream" {
  type        = string
  default     = "ghcr.io"
  description = "Upstream OCI registry the proxy forwards metadata requests to."
}

variable "repository_prefix" {
  type        = string
  default     = "arkeros/senku"
  description = "Prefix prepended to incoming `/v2/<name>/...` paths when rewriting upstream requests. Combined with `var.upstream` this defines the single repository namespace the proxy fronts."
}
