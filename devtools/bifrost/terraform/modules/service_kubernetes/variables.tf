variable "name" {
  type        = string
  description = "Workload name. Used for Deployment, Service, ServiceAccount, HPA, and the derived GSA account_id."
}

variable "namespace" {
  type        = string
  description = "Kubernetes namespace."
}

variable "project_id" {
  type        = string
  description = "GCP project that owns the runtime service account and Secret Manager secrets."
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
  description = "Container port exposed by the Service and probed by the container."
}

variable "args" {
  type        = list(string)
  default     = []
  description = "Container args. Image entrypoint is preserved."
}

variable "resources" {
  type        = object({ cpu = number, memory = number })
  description = "CPU in cores (float, 0.25 = 250m). Memory in MiB (int, 512 = 512Mi). Only requests.cpu is set (no CPU limit); memory requests == limits."
}

variable "env" {
  type        = map(string)
  default     = {}
  description = "Plain environment variables. Values may reference other Terraform outputs."
}

variable "secret_env" {
  type = map(object({
    project = string
    secret  = string
    version = string
  }))
  default     = {}
  description = "Secret env vars sourced from GCP Secret Manager via an ephemeral block + data_wo. version must be an explicit integer; \"latest\" is rejected (immutability)."
  validation {
    condition     = alltrue([for k, v in var.secret_env : v.version != "latest" && can(regex("^[0-9]+$", v.version))])
    error_message = "secret_env[*].version must be an explicit integer; \"latest\" is forbidden."
  }
}

variable "autoscaling" {
  type = object({
    min                    = number
    max                    = number
    target_cpu_utilization = optional(number, 80)
  })
  description = "HPA bounds. min must be >= 1 (HPA requirement); no silent clamping."
  validation {
    condition     = var.autoscaling.min >= 1 && var.autoscaling.max >= var.autoscaling.min
    error_message = "autoscaling.min must be >= 1 (HPA requirement) and autoscaling.max must be >= autoscaling.min."
  }
}

variable "probes" {
  type = object({
    startup_path   = optional(string)
    liveness_path  = optional(string)
    readiness_path = optional(string)
  })
  default     = {}
  description = "HTTP GET probe paths. Omit a field to skip that probe. readiness_path is how pods signal \"not ready for traffic right now\" — important for graceful shutdown and slow warmups."
}

variable "vpa_enabled" {
  type        = bool
  default     = true
  description = "Emit a VerticalPodAutoscaler in InPlaceOrRecreate mode, scoped to memory on the \"app\" container. VPA actively resizes memory at runtime. Requires the VPA CRD to be installed in the cluster; set false if unavailable."
}

variable "service_account_id" {
  type        = string
  default     = null
  description = "GSA account_id for the runtime identity. Defaults to \"svc-<name>\"."
}

variable "labels" {
  type        = map(string)
  default     = {}
  description = "Labels applied to Deployment, Service, and ServiceAccount."
}
