variable "name" {
  type        = string
  description = "CronJob name. Used for the K8s CronJob, ServiceAccount, and derived GSA account_id."
}

variable "project_id" {
  type        = string
  description = "GCP project that owns the runtime service account and Secret Manager secrets."
}

variable "namespace" {
  type        = string
  description = "Kubernetes namespace."
}

variable "image" {
  type        = string
  description = "Container image, digest-pinned."
  validation {
    condition     = can(regex("@sha256:[0-9a-f]{64}$", var.image))
    error_message = "image must be digest-pinned: <registry>/<repo>@sha256:<64-hex>"
  }
}

variable "args" {
  type        = list(string)
  default     = []
  description = "Container args."
}

variable "resources" {
  type        = object({ cpu = number, memory = number })
  description = "CPU in cores (float). Memory in MiB (int). CronJobs set BOTH requests and limits on CPU and memory — the web-serving \"requests only\" rule does not apply to batch. A misbehaving batch job with no CPU limit will starve co-tenant web pods on the node."
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
  description = "Secret env vars from GCP Secret Manager via ephemeral + data_wo. Version must be an integer; \"latest\" is rejected — deploys are immutable and reference a specific SM version."
  validation {
    condition     = alltrue([for k, v in var.secret_env : v.version != "latest" && can(regex("^[0-9]+$", v.version))])
    error_message = "secret_env[*].version must be an explicit integer; \"latest\" is forbidden."
  }
}

variable "schedule" {
  type        = object({ cron = string, time_zone = string })
  description = "Cron spec: cron e.g. \"0 * * * *\"; time_zone e.g. \"Europe/Madrid\"."
}

variable "job" {
  type = object({
    parallelism     = optional(number, 1)
    completions     = optional(number, 1)
    max_retries     = optional(number, 3)
    timeout_seconds = optional(number, 600)
  })
  default     = {}
  description = "Job execution settings."
}

variable "service_account_id" {
  type        = string
  default     = null
  description = "GSA account_id for the runtime identity. Defaults to \"crj-<name>\"."
}

variable "labels" {
  type        = map(string)
  default     = {}
  description = "Labels applied to all K8s objects."
}
