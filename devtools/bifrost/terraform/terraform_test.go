package terraform

import (
	"testing"

	btesting "github.com/arkeros/senku/devtools/bifrost/testing"
	"github.com/arkeros/senku/testing/golden"
)

func TestRenderService(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/service.golden.tf")
}

func TestRenderServiceCloudRunOnly(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service_cloudrun_only.yaml", "testdata/environment_cloudrun_only.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/service_cloudrun_only.golden.tf")
}

func TestRenderCronJob(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/cronjob.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/cronjob.golden.tf")
}
