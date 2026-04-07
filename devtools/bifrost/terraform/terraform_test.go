package terraform

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
	golden.Compare(t, got, "testdata/service.golden.tf")
}

func TestRenderServiceCloudRunOnly(t *testing.T) {
	t.Parallel()

	spec, env, err := loadFixtures("service_cloudrun_only.yaml", "environment_cloudrun_only.yaml")
	if err != nil {
		t.Fatalf("load error = %v", err)
	}
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/service_cloudrun_only.golden.tf")
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
	golden.Compare(t, got, "testdata/cronjob.golden.tf")
}
