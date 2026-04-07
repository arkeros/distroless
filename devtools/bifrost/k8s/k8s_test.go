package k8s

import (
	"os"
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/testing/golden"
)

func loadFixtures(workloadName, envName string) (bifrost.Workload, bifrost.Environment, error) {
	envData, err := os.ReadFile("testdata/" + envName)
	if err != nil {
		return bifrost.Workload{}, bifrost.Environment{}, err
	}
	env, err := bifrost.ParseEnvironment(strings.NewReader(string(envData)))
	if err != nil {
		return bifrost.Workload{}, bifrost.Environment{}, err
	}
	data, err := os.ReadFile("testdata/" + workloadName)
	if err != nil {
		return bifrost.Workload{}, bifrost.Environment{}, err
	}
	w, err := bifrost.Parse(strings.NewReader(string(data)), env)
	if err != nil {
		return bifrost.Workload{}, bifrost.Environment{}, err
	}
	return w, env, nil
}

func TestRenderService(t *testing.T) {
	t.Parallel()

	spec, env, err := loadFixtures("service.yaml", "environment.yaml")
	if err != nil {
		t.Fatalf("load error = %v", err)
	}
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/service.golden.yaml")
}

func TestRenderServiceRequiresKubernetes(t *testing.T) {
	t.Parallel()

	spec, env, err := loadFixtures("service_cloudrun_only.yaml", "environment_cloudrun_only.yaml")
	if err != nil {
		t.Fatalf("load error = %v", err)
	}
	_, err = Render(spec, env)
	if err == nil {
		t.Fatal("Render() should return error when kubernetes is not set")
	}
	if !strings.Contains(err.Error(), "kubernetes") {
		t.Fatalf("error should mention kubernetes, got: %v", err)
	}
}

func TestRenderCronJob(t *testing.T) {
	t.Parallel()

	spec, env, err := loadFixtures("cronjob.yaml", "environment.yaml")
	if err != nil {
		t.Fatalf("load error = %v", err)
	}
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/cronjob.golden.yaml")
}
