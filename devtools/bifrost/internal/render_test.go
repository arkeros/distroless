package internal

import (
	"testing"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
)

func TestResolveSecretFiles_CrossProjectSameName(t *testing.T) {
	t.Parallel()

	secrets := []bifrost.SecretFile{
		{Secret: "shared", Project: "proj-a", Version: 1, Path: "/run/secrets/a.json"},
		{Secret: "shared", Project: "proj-b", Version: 1, Path: "/run/secrets/b.json"},
	}
	res := ResolveSecretFiles("default-project", secrets)

	if len(res.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(res.Volumes))
	}
	if len(res.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(res.Mounts))
	}

	// Volume names must be unique.
	if res.Volumes[0].Name == res.Volumes[1].Name {
		t.Errorf("volume names must be unique, both are %q", res.Volumes[0].Name)
	}
	// Mount names must match their volumes.
	if res.Mounts[0].Name != res.Volumes[0].Name {
		t.Errorf("mount[0].Name = %q, want %q", res.Mounts[0].Name, res.Volumes[0].Name)
	}
	if res.Mounts[1].Name != res.Volumes[1].Name {
		t.Errorf("mount[1].Name = %q, want %q", res.Mounts[1].Name, res.Volumes[1].Name)
	}
}
