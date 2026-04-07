package bifrost_test

import (
	"strings"
	"testing"

	btesting "github.com/arkeros/senku/devtools/bifrost/testing"
)

func TestParse_RejectsUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := btesting.ParseWorkload(t, "testdata/service_unknown_field.yaml", btesting.ValidEnvironment())
	if err == nil {
		t.Fatal("expected error for unknown field servceAccountName, got nil")
	}
	if !strings.Contains(err.Error(), "servceAccountName") {
		t.Fatalf("error should mention the unknown field, got: %v", err)
	}
}

func TestParse_RejectsGCPField(t *testing.T) {
	t.Parallel()
	_, err := btesting.ParseWorkload(t, "testdata/service_gcp_field.yaml", btesting.ValidEnvironment())
	if err == nil {
		t.Fatal("expected error for gcp field in workload, got nil")
	}
	if !strings.Contains(err.Error(), "gcp") {
		t.Fatalf("error should mention gcp, got: %v", err)
	}
}

func TestParse_RejectsKubernetesField(t *testing.T) {
	t.Parallel()
	_, err := btesting.ParseWorkload(t, "testdata/service_kubernetes_field.yaml", btesting.ValidEnvironment())
	if err == nil {
		t.Fatal("expected error for kubernetes field in workload, got nil")
	}
	if !strings.Contains(err.Error(), "kubernetes") {
		t.Fatalf("error should mention kubernetes, got: %v", err)
	}
}

func TestParse_SecretFiles(t *testing.T) {
	t.Parallel()
	w, err := btesting.ParseWorkload(t, "testdata/cronjob_secret_files.yaml", btesting.ValidEnvironment())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.Spec.SecretFiles) != 1 {
		t.Fatalf("expected 1 secret file, got %d", len(w.Spec.SecretFiles))
	}
	sf := w.Spec.SecretFiles[0]
	if sf.Secret != "my-secret" {
		t.Errorf("expected secret %q, got %q", "my-secret", sf.Secret)
	}
	if sf.Path != "/run/secrets/env.json" {
		t.Errorf("expected path %q, got %q", "/run/secrets/env.json", sf.Path)
	}
}
