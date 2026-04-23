variable "name" {
  type        = string
  description = "Cronjob name. Used for the Cloud Run Job, the scheduler job, and the derived GSA account_ids."
}

variable "project_id" {
  type        = string
  description = "GCP project that owns the Cloud Run Job, service accounts, and Secret Manager secrets."
}

variable "region" {
  type        = string
  description = "GCP region for the Cloud Run Job and the Cloud Scheduler job."
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
  description = "CPU in cores (float). Memory in MiB (int). Cloud Run Jobs size tasks from limits; both are enforced."
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
  description = "Secret env vars from GCP Secret Manager. Cloud Run references SM natively via secret_key_ref; version must be an explicit integer (\"latest\" is rejected)."
  validation {
    condition     = alltrue([for k, v in var.secret_env : v.version != "latest" && can(regex("^[0-9]+$", v.version))])
    error_message = "secret_env[*].version must be an explicit integer; \"latest\" is forbidden."
  }
}

variable "schedule" {
  type        = object({ cron = string, time_zone = string })
  description = "Cron spec for the Cloud Scheduler trigger."
}

variable "job" {
  type = object({
    parallelism     = optional(number, 1)
    completions     = optional(number, 1)
    max_retries     = optional(number, 3)
    timeout_seconds = optional(number, 600)
  })
  default     = {}
  description = "Cloud Run Job execution settings."
}

variable "cloud_scheduler" {
  type = object({
    retry_count              = optional(number, 0)
    attempt_deadline_seconds = optional(number, null)
  })
  default     = {}
  description = "Cloud Scheduler retry configuration."
}

variable "service_account_id" {
  type        = string
  default     = null
  description = "GSA account_id for the runtime (Cloud Run Job) identity. Defaults to \"crj-<name>\"."
}
