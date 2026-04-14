package k8s

import (
	"strings"
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
	golden.Compare(t, got, "testdata/service.golden.yaml")
}

func TestRenderServiceRequiresKubernetes(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service_cloudrun_only.yaml", "testdata/environment_cloudrun_only.yaml")
	_, err := Render(spec, env)
	if err == nil {
		t.Fatal("Render() should return error when kubernetes is not set")
	}
	if !strings.Contains(err.Error(), "kubernetes") {
		t.Fatalf("error should mention kubernetes, got: %v", err)
	}
}

func TestRenderServiceHA(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service_ha.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/service_ha.golden.yaml")
}

func TestRenderServiceCrossProject(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service_cross_project.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/service_cross_project.golden.yaml")
}

func TestRenderServiceSecretEnv(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service-secret-env.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/service-secret-env.golden.yaml")
}

func TestRenderCronJobSecretEnv(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/cronjob-secret-env.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/cronjob-secret-env.golden.yaml")
}

func TestRenderCronJob(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/cronjob.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/cronjob.golden.yaml")
}
