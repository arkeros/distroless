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

func TestResolveSecretFiles_VersionAsKey(t *testing.T) {
	t.Parallel()

	// For Cloud Run, the volume item key is the Secret Manager version
	// and the path is the filename. This is correct for the Cloud Run API.
	secrets := []bifrost.SecretFile{
		{Secret: "registry-env", Version: 1, Path: "/run/secrets/env.json"},
	}
	res := ResolveSecretFiles("my-project", secrets)

	if len(res.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(res.Volumes))
	}

	items := res.Volumes[0].VolumeSource.Secret.Items
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Key != "1" {
		t.Errorf("item key = %q, want %q", items[0].Key, "1")
	}
	if items[0].Path != "env.json" {
		t.Errorf("item path = %q, want %q", items[0].Path, "env.json")
	}
}
