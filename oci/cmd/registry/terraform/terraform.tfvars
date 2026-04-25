project_id = "senku-prod"

# `image` is NOT set here. It's materialized into `image.auto.tfvars.json` by
# `bazel run //oci/cmd/registry:image_tfvars` and auto-loaded by Terraform.
# Running `terraform apply` without that step first errors "variable image
# has no value" — intentional, forces the deploy flow.

regions = {
  usc1 = "us-central1"
  euw3 = "europe-west3"
  ane1 = "asia-northeast1"
  sae1 = "southamerica-east1"
  ase1 = "australia-southeast1"
}
