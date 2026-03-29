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

```bash
terraform init
terraform apply
```
