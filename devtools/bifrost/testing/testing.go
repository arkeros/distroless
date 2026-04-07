// Package testing provides test helpers for bifrost workload and environment parsing.
package testing

import (
	"os"
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
)

// ValidEnvironment returns a minimal environment suitable for tests.
func ValidEnvironment() bifrost.Environment {
	return bifrost.Environment{
		APIVersion: bifrost.APIVersion,
		Kind:       bifrost.KindEnvironment,
		Metadata:   bifrost.ObjectMeta{Name: "test-env"},
		Spec: bifrost.EnvironmentSpec{
			GCP: bifrost.GCPSpec{
				ProjectID:     "my-project",
				ProjectNumber: "123456",
				Region:        "us-central1",
			},
		},
	}
}

// LoadFixtures parses a workload and environment from testdata files.
func LoadFixtures(t *testing.T, workloadFile, envFile string) (bifrost.Workload, bifrost.Environment) {
	t.Helper()
	env := LoadEnvironment(t, envFile)
	w := LoadWorkload(t, workloadFile, env)
	return w, env
}

// LoadEnvironment parses an environment from a testdata file.
func LoadEnvironment(t *testing.T, path string) bifrost.Environment {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	env, err := bifrost.ParseEnvironment(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return env
}

// LoadWorkload parses a workload from a testdata file.
func LoadWorkload(t *testing.T, path string, env bifrost.Environment) bifrost.Workload {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	w, err := bifrost.Parse(strings.NewReader(string(data)), env)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return w
}

// ParseWorkload attempts to parse a workload file and returns the error.
// Use this for tests that expect parse failures.
func ParseWorkload(t *testing.T, path string, env bifrost.Environment) (bifrost.Workload, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return bifrost.Parse(strings.NewReader(string(data)), env)
}
