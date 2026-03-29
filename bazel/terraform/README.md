# Bazel Infrastructure

Terraform for Bazel remote cache and GitHub Actions CI authentication.

## Bootstrap (one-time)

```bash
gcloud projects create senku-prod --name="senku"
gcloud billing projects link senku-prod --billing-account=BILLING_ID
gcloud services enable storage.googleapis.com iam.googleapis.com iamcredentials.googleapis.com sts.googleapis.com --project=senku-prod
gcloud storage buckets create gs://senku-prod-terraform-state --location=US --uniform-bucket-level-access --project=senku-prod
```

## Apply

First apply must be run locally since the GitHub Actions SA doesn't have
the IAM roles it needs until Terraform creates them.

```bash
terraform init
terraform apply
```
