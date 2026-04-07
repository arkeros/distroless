package bifrost

import (
	"strings"
	"testing"
)

const validEnvironmentYAML = `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Environment
metadata:
  name: senku-prod
spec:
  gcp:
    projectId: senku-prod
    projectNumber: "874944788122"
    region: europe-west1
  kubernetes:
    namespace: jobs
`

func TestParseEnvironment_Valid(t *testing.T) {
	t.Parallel()

	env, err := ParseEnvironment(strings.NewReader(validEnvironmentYAML))
	if err != nil {
		t.Fatalf("ParseEnvironment() error = %v", err)
	}
	if got, want := env.Metadata.Name, "senku-prod"; got != want {
		t.Errorf("Metadata.Name = %q, want %q", got, want)
	}
	if got, want := env.Spec.GCP.ProjectID, "senku-prod"; got != want {
		t.Errorf("GCP.ProjectID = %q, want %q", got, want)
	}
	if got, want := env.Spec.GCP.ProjectNumber, "874944788122"; got != want {
		t.Errorf("GCP.ProjectNumber = %q, want %q", got, want)
	}
	if got, want := env.Spec.GCP.Region, "europe-west1"; got != want {
		t.Errorf("GCP.Region = %q, want %q", got, want)
	}
	if env.Spec.Kubernetes == nil {
		t.Fatal("Kubernetes should not be nil")
	}
	if got, want := env.Spec.Kubernetes.Namespace, "jobs"; got != want {
		t.Errorf("Kubernetes.Namespace = %q, want %q", got, want)
	}
	// defaults
	if got, want := env.Spec.Kubernetes.ServiceType, "ClusterIP"; got != want {
		t.Errorf("Kubernetes.ServiceType = %q, want %q (should default)", got, want)
	}
}

func TestParseEnvironment_RejectsWrongKind(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Service
metadata:
  name: senku-prod
spec:
  gcp:
    projectId: senku-prod
    projectNumber: "874944788122"
    region: europe-west1
`
	_, err := ParseEnvironment(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("expected error for wrong kind, got nil")
	}
	if !strings.Contains(err.Error(), "Environment") {
		t.Fatalf("error should mention Environment, got: %v", err)
	}
}

func TestParseEnvironment_RejectsUnknownFields(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Environment
metadata:
  name: senku-prod
spec:
  gcp:
    projectId: senku-prod
    projectNumber: "874944788122"
    region: europe-west1
  unknownField: bad
`
	_, err := ParseEnvironment(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknownField") {
		t.Fatalf("error should mention unknownField, got: %v", err)
	}
}

func TestParseEnvironment_RequiresGCPFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		yaml       string
		errContain string
	}{
		{
			name: "missing projectId",
			yaml: `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Environment
metadata:
  name: test
spec:
  gcp:
    projectNumber: "123"
    region: us-central1
`,
			errContain: "projectId",
		},
		{
			name: "missing projectNumber",
			yaml: `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Environment
metadata:
  name: test
spec:
  gcp:
    projectId: test
    region: us-central1
`,
			errContain: "projectNumber",
		},
		{
			name: "missing region",
			yaml: `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Environment
metadata:
  name: test
spec:
  gcp:
    projectId: test
    projectNumber: "123"
`,
			errContain: "region",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseEnvironment(strings.NewReader(tt.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("error should contain %q, got: %v", tt.errContain, err)
			}
		})
	}
}
