package main

import (
	"os"
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
)

func TestParseServiceSpecValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	_, err := bifrost.Parse(strings.NewReader(`
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Service
metadata:
  name: broken
spec:
  image: registry
  port: 8080
  resources:
    limits:
      cpu: 1000m
      memory: 256Mi
  gcp:
    region: europe-west3
`))
	if err == nil || (!strings.Contains(err.Error(), "projectId") && !strings.Contains(err.Error(), "projectNumber")) {
		t.Fatalf("ParseServiceSpec() error = %v, want project validation", err)
	}
}

func TestParseServiceSpecAppliesDefaults(t *testing.T) {
	t.Parallel()

	spec, err := loadSpecFixture("service.yaml")
	if err != nil {
		t.Fatalf("ParseServiceSpec() error = %v", err)
	}

	if got, want := spec.Spec.Autoscaling.Concurrency, int64(80); got != want {
		t.Fatalf("spec.Spec.Autoscaling.Concurrency = %d, want %d", got, want)
	}
	if got, want := spec.Spec.Autoscaling.TargetCPUUtilization, int32(80); got != want {
		t.Fatalf("spec.Spec.Autoscaling.TargetCPUUtilization = %d, want %d", got, want)
	}
	if spec.Spec.Kubernetes == nil {
		t.Fatal("spec.Spec.Kubernetes should not be nil when kubernetes is set")
	}
	if got, want := spec.Spec.Kubernetes.ServiceType, "ClusterIP"; got != want {
		t.Fatalf("spec.Spec.Kubernetes.ServiceType = %q, want %q", got, want)
	}
	if got, want := spec.Spec.Kubernetes.Namespace, "default"; got != want {
		t.Fatalf("spec.Spec.Kubernetes.Namespace = %q, want %q", got, want)
	}
	if got, want := spec.Spec.ServiceAccountName, "svc-registry@senku-prod.iam.gserviceaccount.com"; got != want {
		t.Fatalf("spec.Spec.ServiceAccountName = %q, want %q", got, want)
	}
}

func TestParseCronJobAppliesDefaults(t *testing.T) {
	t.Parallel()

	spec, err := bifrost.Parse(strings.NewReader(`
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: CronJob
metadata:
  name: daily-report
spec:
  image: report
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
  schedule:
    cron: "0 1 * * *"
    timeZone: Europe/Madrid
  gcp:
    projectId: senku-prod
    projectNumber: "874944788122"
    region: europe-west1
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := spec.Spec.ServiceAccountName, "crj-daily-report@senku-prod.iam.gserviceaccount.com"; got != want {
		t.Fatalf("spec.Spec.ServiceAccountName = %q, want %q", got, want)
	}
	if got, want := spec.Spec.Job.Parallelism, int32(1); got != want {
		t.Fatalf("spec.Spec.Job.Parallelism = %d, want %d", got, want)
	}
	if got, want := spec.Spec.Job.Completions, int32(1); got != want {
		t.Fatalf("spec.Spec.Job.Completions = %d, want %d", got, want)
	}
	if got, want := spec.Spec.Job.MaxRetries, int32(3); got != want {
		t.Fatalf("spec.Spec.Job.MaxRetries = %d, want %d", got, want)
	}
	if got, want := spec.Spec.Job.TimeoutSeconds, int64(600); got != want {
		t.Fatalf("spec.Spec.Job.TimeoutSeconds = %d, want %d", got, want)
	}
	if got, want := spec.Spec.GCP.Region, "europe-west1"; got != want {
		t.Fatalf("spec.Spec.GCP.Region = %q, want %q", got, want)
	}
}

func loadSpecFixture(name string) (bifrost.Workload, error) {
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		return bifrost.Workload{}, err
	}
	return bifrost.Parse(strings.NewReader(string(data)))
}
