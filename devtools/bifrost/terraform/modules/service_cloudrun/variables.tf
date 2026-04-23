variable "name" {
  type        = string
  description = "Cloud Run service name. Used for the service, the derived runtime GSA account_id, and the revision name prefix."
}

variable "project_id" {
  type        = string
  description = "GCP project that owns the Cloud Run service, GSA, and Secret Manager secrets."
}

variable "region" {
  type        = string
  description = "GCP region for the Cloud Run service."
}

variable "image" {
  type        = string
  description = "Container image, digest-pinned."
  validation {
    condition     = can(regex("@sha256:[0-9a-f]{64}$", var.image))
    error_message = "image must be digest-pinned: <registry>/<repo>@sha256:<64-hex>"
  }
}

variable "port" {
  type        = number
  default     = 8080
  description = "Container port exposed to the Cloud Run front-end."
}

variable "args" {
  type        = list(string)
  default     = []
  description = "Container args. Image entrypoint is preserved."
}

variable "resources" {
  type        = object({ cpu = number, memory = number })
  description = "CPU in cores (float, e.g. 0.5). Memory in MiB (int, e.g. 512). Cloud Run uses these as limits; there is no request/limit split in Cloud Run's model."
}

variable "env" {
  type        = map(string)
  default     = {}
  description = "Plain environment variables."
}

variable "secret_env" {
  type = map(object({
    project = string
    secret  = string
    version = string
  }))
  default     = {}
  description = "Secret env vars from GCP Secret Manager, resolved natively by Cloud Run via secret_key_ref at startup. Version must be an explicit integer; \"latest\" is rejected (immutability). No ephemeral/data_wo needed — Cloud Run never materialises a K8s Secret."
  validation {
    condition     = alltrue([for k, v in var.secret_env : v.version != "latest" && can(regex("^[0-9]+$", v.version))])
    error_message = "secret_env[*].version must be an explicit integer; \"latest\" is forbidden."
  }
}

variable "scaling" {
  type = object({
    min = number
    max = number
  })
  description = "Instance scaling bounds. Cloud Run allows min = 0 (scale-to-zero); max must be >= min."
  validation {
    condition     = var.scaling.min >= 0 && var.scaling.max >= var.scaling.min
    error_message = "scaling.min must be >= 0 and scaling.max must be >= scaling.min."
  }
}

variable "concurrency" {
  type        = number
  default     = 80
  description = "Max concurrent requests per instance. Cloud Run's native primitive for load-per-instance; there's no K8s equivalent."
}

variable "timeout_seconds" {
  type        = number
  default     = 300
  description = "Request timeout. Cloud Run kills the request — not the instance — on expiry."
}

variable "probes" {
  type = object({
    startup_path  = optional(string)
    liveness_path = optional(string)
  })
  default     = {}
  description = "HTTP GET probe paths. Cloud Run doesn't have a readiness concept (it routes by health, not by readiness)."
}

variable "ingress" {
  type        = string
  default     = "INGRESS_TRAFFIC_ALL"
  description = "Who can reach the Cloud Run URL. Valid: INGRESS_TRAFFIC_ALL, INGRESS_TRAFFIC_INTERNAL_ONLY, INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER."
  validation {
    condition     = contains(["INGRESS_TRAFFIC_ALL", "INGRESS_TRAFFIC_INTERNAL_ONLY", "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"], var.ingress)
    error_message = "ingress must be INGRESS_TRAFFIC_ALL, INGRESS_TRAFFIC_INTERNAL_ONLY, or INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER."
  }
}

variable "public" {
  type        = bool
  default     = false
  description = "When true, grant roles/run.invoker to allUsers. Combined with ingress = INGRESS_TRAFFIC_ALL this makes the service publicly callable without auth. Defaults off — private by construction."
}

variable "cpu_idle" {
  type        = bool
  default     = true
  description = "Whether CPU is throttled outside requests. true = cheaper (CPU allocated only during requests), default; false = CPU always allocated (faster cold starts for background work, more expensive)."
}

variable "startup_cpu_boost" {
  type        = bool
  default     = true
  description = "Extra CPU during container startup to reduce cold-start latency. Costs nothing extra."
}

variable "execution_environment" {
  type        = string
  default     = "EXECUTION_ENVIRONMENT_GEN2"
  description = "Cloud Run execution environment. GEN2 gives Linux cgroups and more realistic Linux semantics; GEN1 is the legacy gVisor sandbox."
  validation {
    condition     = contains(["EXECUTION_ENVIRONMENT_GEN1", "EXECUTION_ENVIRONMENT_GEN2"], var.execution_environment)
    error_message = "execution_environment must be EXECUTION_ENVIRONMENT_GEN1 or EXECUTION_ENVIRONMENT_GEN2."
  }
}

variable "service_account_id" {
  type        = string
  default     = null
  description = "GSA account_id for the runtime identity. Defaults to \"svc-<name>\"."
}

variable "labels" {
  type        = map(string)
  default     = {}
  description = "Labels applied to the Cloud Run service."
}
