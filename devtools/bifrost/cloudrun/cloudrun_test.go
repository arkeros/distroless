package cloudrun

import (
	"os"
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
)

func loadSpecFixture(name string) (bifrost.Workload, error) {
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		return bifrost.Workload{}, err
	}
	return bifrost.Parse(strings.NewReader(string(data)))
}

func TestRenderService(t *testing.T) {
	t.Parallel()

	spec, err := loadSpecFixture("service.yaml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := Render(spec)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for _, want := range []string{
		"apiVersion: serving.knative.dev/v1",
		"kind: Service",
		"name: registry",
		"namespace: \"874944788122\"",
		"serviceAccountName: svc-registry@senku-prod.iam.gserviceaccount.com",
		"containerConcurrency: 80",
		"minScale: \"0\"",
		"run.googleapis.com/execution-environment: gen2",
		"requests:",
		"cpu: 250m",
		"run.googleapis.com/ingress: all",
		"secretName: registry-env",
		"mountPath: /run/secrets",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "\nstatus:") {
		t.Fatalf("output should not contain status\n%s", got)
	}
	if strings.Contains(string(got), "securityContext") {
		t.Fatalf("output should not contain securityContext\n%s", got)
	}
}

func TestRenderCronJob(t *testing.T) {
	t.Parallel()

	spec, err := loadSpecFixture("cronjob.yaml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := Render(spec)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for _, want := range []string{
		"apiVersion: run.googleapis.com/v1",
		"kind: Job",
		"name: analytics-data-export",
		"namespace: \"874944788122\"",
		"cloud.googleapis.com/location: europe-west1",
		"run.googleapis.com/execution-environment: gen2",
		"parallelism: 1",
		"taskCount: 1",
		"maxRetries: 3",
		"timeoutSeconds: \"600\"",
		"serviceAccountName: crj-analytics-data-export@senku-prod.iam.gserviceaccount.com",
		"mountPath: /run/secrets",
		"secretName: data-export-env",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "\nstatus:") {
		t.Fatalf("output should not contain status\n%s", got)
	}
	if strings.Contains(string(got), "securityContext") {
		t.Fatalf("output should not contain securityContext\n%s", got)
	}
}
