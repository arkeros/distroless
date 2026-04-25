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
  type        = map(string)
  description = "Map of short region code (used as the Cloud Run service name suffix and the runtime GSA account_id suffix) → GCP region. The short code is what keeps `svc-registry-<code>` under GCP's 30-char account-id cap for long region names like `southamerica-east1`."
  validation {
    condition     = alltrue([for code in keys(var.regions) : can(regex("^[a-z][a-z0-9]{0,6}$", code))])
    error_message = "region keys must be lowercase alphanumeric, start with a letter, and be at most 7 chars — so `svc-registry-<code>` stays ≤ 20 chars."
  }
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
