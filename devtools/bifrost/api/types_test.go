package bifrost

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func validWorkload() Workload {
	return Workload{
		APIVersion: APIVersion,
		Kind:       KindService,
		Metadata:   ObjectMeta{Name: "test-svc"},
		Spec: Spec{
			Image: "test-image",
			Port:  8080,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			GCP: GCPSpec{
				ProjectID:     "my-project",
				ProjectNumber: "123456",
				Region:        "us-central1",
			},
		},
	}
}

func TestValidate_Port(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    int32
		wantErr bool
	}{
		{"valid port", 8080, false},
		{"min valid port", 1, false},
		{"max valid port", 65535, false},
		{"zero port", 0, true},
		{"negative port", -1, true},
		{"port above 65535", 65536, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := validWorkload()
			w.Spec.Port = tt.port
			err := w.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "spec.port") {
				t.Fatalf("error should mention spec.port, got: %v", err)
			}
		})
	}
}

func TestParse_RejectsUnknownFields(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: Service
metadata:
  name: test-svc
spec:
  image: test-image
  port: 8080
  servceAccountName: typo@my-project.iam.gserviceaccount.com
  resources:
    limits:
      cpu: "1"
      memory: 256Mi
  gcp:
    projectId: my-project
    projectNumber: "123456"
    region: us-central1
`
	_, err := Parse(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("expected error for unknown field servceAccountName, got nil")
	}
	if !strings.Contains(err.Error(), "servceAccountName") {
		t.Fatalf("error should mention the unknown field, got: %v", err)
	}
}

func TestValidate_CloudRunIngress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ingress string
		wantErr bool
	}{
		{"empty defaults to all", "", false},
		{"all", "all", false},
		{"internal", "internal", false},
		{"internal-and-cloud-load-balancing", "internal-and-cloud-load-balancing", false},
		{"invalid value", "public", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := validWorkload()
			w.Spec.GCP.CloudRun.Ingress = tt.ingress
			err := w.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "ingress") {
				t.Fatalf("error should mention ingress, got: %v", err)
			}
		})
	}
}

func TestValidate_RequestsExceedLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reqCPU     string
		limCPU     string
		reqMem     string
		limMem     string
		wantErr    bool
		errContain string
	}{
		{
			name:   "requests equal limits",
			reqCPU: "1", limCPU: "1",
			reqMem: "256Mi", limMem: "256Mi",
			wantErr: false,
		},
		{
			name:   "requests below limits",
			reqCPU: "500m", limCPU: "1",
			reqMem: "128Mi", limMem: "256Mi",
			wantErr: false,
		},
		{
			name:   "cpu request exceeds limit",
			reqCPU: "2", limCPU: "1",
			reqMem: "256Mi", limMem: "256Mi",
			wantErr:    true,
			errContain: "spec.resources.requests.cpu",
		},
		{
			name:   "memory request exceeds limit",
			reqCPU: "1", limCPU: "1",
			reqMem: "512Mi", limMem: "256Mi",
			wantErr:    true,
			errContain: "spec.resources.requests.memory",
		},
		{
			name:   "cpu equal in different units",
			reqCPU: "2000m", limCPU: "2",
			reqMem: "256Mi", limMem: "256Mi",
			wantErr: false,
		},
		{
			name:   "cpu exceeds in millicore units",
			reqCPU: "2001m", limCPU: "2",
			reqMem: "256Mi", limMem: "256Mi",
			wantErr:    true,
			errContain: "spec.resources.requests.cpu",
		},
		{
			name:   "memory exceeds across units",
			reqCPU: "1", limCPU: "1",
			reqMem: "2048Mi", limMem: "1Gi",
			wantErr:    true,
			errContain: "spec.resources.requests.memory",
		},
		{
			name:   "memory equal across units",
			reqCPU: "1", limCPU: "1",
			reqMem: "1024Mi", limMem: "1Gi",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := validWorkload()
			w.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(tt.reqCPU),
				corev1.ResourceMemory: resource.MustParse(tt.reqMem),
			}
			w.Spec.Resources.Limits = corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(tt.limCPU),
				corev1.ResourceMemory: resource.MustParse(tt.limMem),
			}
			err := w.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("error should mention %q, got: %v", tt.errContain, err)
			}
		})
	}
}

func TestParse_SecretFiles(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: bifrost.apotema.cloud/v1alpha1
kind: CronJob
metadata:
  name: test-job
spec:
  image: test-image
  resources:
    limits:
      cpu: "1"
      memory: 256Mi
  secretFiles:
    - secret: my-secret
      path: /run/secrets/env.json
  schedule:
    cron: "0 12 * * *"
  gcp:
    projectId: my-project
    projectNumber: "123456"
    region: us-central1
`
	w, err := Parse(strings.NewReader(yaml))
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

func TestSecretFile_ParseSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		secret         string
		defaultProject string
		wantProject    string
		wantName       string
		wantVersion    string
	}{
		{"bare name", "data-export-env", "my-project", "my-project", "data-export-env", "latest"},
		{"full path no version", "projects/other/secrets/foo", "my-project", "other", "foo", "latest"},
		{"full path with version", "projects/other/secrets/foo/versions/4", "my-project", "other", "foo", "4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sf := SecretFile{Secret: tt.secret}
			proj, name, ver := sf.ParseSecret(tt.defaultProject)
			if proj != tt.wantProject {
				t.Errorf("project = %q, want %q", proj, tt.wantProject)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if ver != tt.wantVersion {
				t.Errorf("version = %q, want %q", ver, tt.wantVersion)
			}
		})
	}
}

func TestValidate_SecretFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		secretFiles []SecretFile
		wantErr     bool
		errContain  string
	}{
		{"valid", []SecretFile{{Secret: "foo", Path: "/run/secrets/a.json"}}, false, ""},
		{"empty secret", []SecretFile{{Secret: "", Path: "/run/secrets/a.json"}}, true, "secret is required"},
		{"empty path", []SecretFile{{Secret: "foo", Path: ""}}, true, "path is required"},
		{"relative path", []SecretFile{{Secret: "foo", Path: "relative/path"}}, true, "must be absolute"},
		{"malformed projects path", []SecretFile{{Secret: "projects//secrets/", Path: "/a"}}, true, "not a valid"},
		{"malformed projects no secrets", []SecretFile{{Secret: "projects/p", Path: "/a"}}, true, "not a valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := validWorkload()
			w.Kind = KindCronJob
			w.Spec.SecretFiles = tt.secretFiles
			w.Spec.Schedule.Cron = "0 * * * *"
			err := w.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("error should contain %q, got: %v", tt.errContain, err)
			}
		})
	}
}

