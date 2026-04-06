package k8s

import (
	"os"
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/testing/golden"
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
	golden.Compare(t, got, "testdata/service.golden.yaml")
}

func TestRenderServiceRequiresKubernetes(t *testing.T) {
	t.Parallel()

	spec, err := loadSpecFixture("service_cloudrun_only.yaml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	_, err = Render(spec)
	if err == nil {
		t.Fatal("Render() should return error when kubernetes is not set")
	}
	if !strings.Contains(err.Error(), "kubernetes") {
		t.Fatalf("error should mention kubernetes, got: %v", err)
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
	golden.Compare(t, got, "testdata/cronjob.golden.yaml")
}
