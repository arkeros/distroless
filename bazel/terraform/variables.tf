variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCS bucket region"
  type        = string
  default     = "US"
}
