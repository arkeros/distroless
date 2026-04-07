package main

import (
	"os"
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
)

func loadEnvFixture(name string) (bifrost.Environment, error) {
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		return bifrost.Environment{}, err
	}
	return bifrost.ParseEnvironment(strings.NewReader(string(data)))
}

func loadSpecFixture(name string, env bifrost.Environment) (bifrost.Workload, error) {
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		return bifrost.Workload{}, err
	}
	return bifrost.Parse(strings.NewReader(string(data)), env)
}

func TestParseServiceSpecAppliesDefaults(t *testing.T) {
	t.Parallel()

	env, err := loadEnvFixture("environment.yaml")
	if err != nil {
		t.Fatalf("ParseEnvironment() error = %v", err)
	}
	spec, err := loadSpecFixture("service.yaml", env)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got, want := spec.Spec.Autoscaling.Concurrency, int64(80); got != want {
		t.Fatalf("spec.Spec.Autoscaling.Concurrency = %d, want %d", got, want)
	}
	if got, want := spec.Spec.Autoscaling.TargetCPUUtilization, int32(80); got != want {
		t.Fatalf("spec.Spec.Autoscaling.TargetCPUUtilization = %d, want %d", got, want)
	}
	if got, want := env.Spec.Kubernetes.ServiceType, "ClusterIP"; got != want {
		t.Fatalf("env.Spec.Kubernetes.ServiceType = %q, want %q", got, want)
	}
	if got, want := env.Spec.Kubernetes.Namespace, "jobs"; got != want {
		t.Fatalf("env.Spec.Kubernetes.Namespace = %q, want %q", got, want)
	}
	if got, want := spec.Spec.ServiceAccountName, "svc-registry@senku-prod.iam.gserviceaccount.com"; got != want {
		t.Fatalf("spec.Spec.ServiceAccountName = %q, want %q", got, want)
	}
}

func TestParseCronJobAppliesDefaults(t *testing.T) {
	t.Parallel()

	env, err := loadEnvFixture("environment.yaml")
	if err != nil {
		t.Fatalf("ParseEnvironment() error = %v", err)
	}

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
`), env)
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
	if got, want := env.Spec.GCP.Region, "europe-west1"; got != want {
		t.Fatalf("env.Spec.GCP.Region = %q, want %q", got, want)
	}
}
