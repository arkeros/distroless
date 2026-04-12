package cloudrun

import (
	"strings"
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
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

func TestRenderCronJob(t *testing.T) {
	t.Parallel()

	spec, env := btesting.LoadFixtures(t, "testdata/cronjob.yaml", "testdata/environment.yaml")
	got, err := Render(spec, env)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	golden.Compare(t, got, "testdata/cronjob.golden.yaml")
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
