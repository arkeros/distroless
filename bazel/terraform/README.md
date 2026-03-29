# Bazel Infrastructure

Terraform for Bazel remote cache and GitHub Actions CI authentication.

## Bootstrap (one-time)

```bash
gcloud projects create senku-prod --name="senku"
gcloud billing projects link senku-prod --billing-account=BILLING_ID
gcloud services enable storage.googleapis.com iam.googleapis.com iamcredentials.googleapis.com sts.googleapis.com cloudresourcemanager.googleapis.com --project=senku-prod
gcloud storage buckets create gs://senku-prod-terraform-state --location=US --uniform-bucket-level-access --project=senku-prod
```

## Apply

```bash
terraform init
terraform apply
```

After the first apply, grant the GitHub Actions SA the roles it needs to
run Terraform in CI (not managed in Terraform to avoid circular dependency):

```bash
SA="github-actions-senku@senku-prod.iam.gserviceaccount.com"
for role in roles/storage.admin roles/iam.workloadIdentityPoolAdmin roles/iam.serviceAccountAdmin; do
  gcloud projects add-iam-policy-binding senku-prod \
    --member="serviceAccount:$SA" --role="$role"
done
```
