package cloudrun

import (
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	btesting "github.com/arkeros/senku/devtools/bifrost/testing"
	"github.com/arkeros/senku/testing/golden"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

func TestRenderCronJob(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/cronjob.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/cronjob.golden.yaml")
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

// Cloud Run rejects fractional CPU limits when containerConcurrency > 1; the
// render should fail at build time rather than at `gcloud run services replace`.
func TestRenderService_RejectsFractionalCPUWithDefaultConcurrency(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service.yaml", "testdata/environment.yaml")
	spec.Spec.Resources.Limits[corev1.ResourceCPU] = resource.MustParse("500m")
	_, err := Render(spec, env)
	if err == nil {
		t.Fatal("expected error for fractional cpu with concurrency > 1")
	}
	if !strings.Contains(err.Error(), "concurrency") {
		t.Errorf("error should mention concurrency, got: %v", err)
	}
}

// Fractional CPU is fine when concurrency is 1 — only the > 1 case trips.
func TestRenderService_AllowsFractionalCPUWhenConcurrencyIsOne(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service.yaml", "testdata/environment.yaml")
	spec.Spec.Resources.Limits[corev1.ResourceCPU] = resource.MustParse("500m")
	spec.Spec.Autoscaling.Concurrency = 1
	if _, err := Render(spec, env); err != nil {
		t.Fatalf("Render() error = %v (fractional cpu should be allowed with concurrency=1)", err)
	}
}

// Cloud Run's gen2 execution environment requires memory >= 512Mi; bifrost
// always emits gen2, so sub-512Mi limits should fail at build time.
func TestRenderService_RejectsSubGen2MemoryFloor(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service.yaml", "testdata/environment.yaml")
	spec.Spec.Resources.Limits[corev1.ResourceMemory] = resource.MustParse("128Mi")
	_, err := Render(spec, env)
	if err == nil {
		t.Fatal("expected error for memory < 512Mi with gen2 execution environment")
	}
	if !strings.Contains(err.Error(), "memory") || !strings.Contains(err.Error(), "gen2") {
		t.Errorf("error should mention memory and gen2, got: %v", err)
	}
}

func TestRenderService_RejectsDuplicateEnvKeys(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/service-secret-env.yaml", "testdata/environment.yaml")
	spec.Spec.Env["API_KEY"] = "override"
	_, err := Render(spec, env)
	if err == nil {
		t.Fatal("expected error for duplicate key across env and secretEnv")
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Errorf("error should mention the key, got: %v", err)
	}
}

func TestResolveSecretEnv_RejectsSpread(t *testing.T) {
	t.Parallel()

	_, _, err := resolveSecretEnv("proj", map[string]string{
		"...db": "gcp:///projects/proj/secrets/s/versions/1",
	})
	if err == nil {
		t.Fatal("expected error for spread on Cloud Run")
	}
	if !strings.Contains(err.Error(), "spread") {
		t.Errorf("error should mention spread, got: %v", err)
	}
}

func TestResolveSecretEnv_RejectsFragment(t *testing.T) {
	t.Parallel()

	_, _, err := resolveSecretEnv("proj", map[string]string{
		"DB_HOST": "gcp:///projects/proj/secrets/s/versions/1#/host",
	})
	if err == nil {
		t.Fatal("expected error for fragment on Cloud Run")
	}
	if !strings.Contains(err.Error(), "fragment") {
		t.Errorf("error should mention fragment, got: %v", err)
	}
}

func TestSecretsAnnotation_CrossProjectSameName(t *testing.T) {
	t.Parallel()

	secrets := []bifrost.SecretFile{
		{Secret: "shared", Project: "proj-a", Version: 1, Path: "/run/secrets/a.json"},
		{Secret: "shared", Project: "proj-b", Version: 1, Path: "/run/secrets/b.json"},
	}
	got := secretsAnnotation("default-project", secrets)

	// Both projects must appear.
	if !strings.Contains(got, "proj-a") {
		t.Errorf("expected proj-a in annotation, got: %s", got)
	}
	if !strings.Contains(got, "proj-b") {
		t.Errorf("expected proj-b in annotation, got: %s", got)
	}

	// Aliases must be unique.
	entries := strings.Split(got, ",")
	if len(entries) != 2 {
		t.Fatalf("expected 2 annotation entries, got %d: %s", len(entries), got)
	}
	alias0 := entries[0][:strings.IndexByte(entries[0], ':')]
	alias1 := entries[1][:strings.IndexByte(entries[1], ':')]
	if alias0 == alias1 {
		t.Errorf("aliases must be unique, both are %q in: %s", alias0, got)
	}
}
