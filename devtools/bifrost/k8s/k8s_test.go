package k8s

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
		"apiVersion: apps/v1",
		"kind: ServiceAccount",
		"kind: Deployment",
		"kind: HorizontalPodAutoscaler",
		"kind: Service",
		"namespace: default",
		"iam.gke.io/gcp-service-account: svc-registry@senku-prod.iam.gserviceaccount.com",
		"serviceAccountName: registry",
		"requests:",
		"cpu: 250m",
		"minReplicas: 1",
		"maxReplicas: 3",
		"averageUtilization: 80",
		"type: ClusterIP",
		"runAsNonRoot: true",
		"allowPrivilegeEscalation: false",
		"readOnlyRootFilesystem: true",
		"- ALL",
		"kind: Secret",
		"name: registry-env-71bb3ea30f",
		"JHtnY3BzbTovLy9wcm9qZWN0cy9zZW5rdS1wcm9kL3NlY3JldHMvcmVnaXN0cnktZW52L3ZlcnNpb25zL2xhdGVzdH0=",
		"secretName: registry-env-71bb3ea30f",
		"mountPath: /run/secrets",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "\nstatus:") {
		t.Fatalf("output should not contain status\n%s", got)
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
		"kind: ServiceAccount",
		"kind: CronJob",
		"namespace: jobs",
		"schedule: 0 12 * * *",
		"timeZone: Europe/Madrid",
		"iam.gke.io/gcp-service-account: crj-analytics-data-export@senku-prod.iam.gserviceaccount.com",
		"restartPolicy: Never",
		"backoffLimit: 3",
		"activeDeadlineSeconds: 600",
		"mountPath: /run/secrets",
		"secretName: data-export-env-4badae36a3",
		"runAsNonRoot: true",
		"allowPrivilegeEscalation: false",
		"readOnlyRootFilesystem: true",
		"- ALL",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "\nstatus:") {
		t.Fatalf("output should not contain status\n%s", got)
	}
}
