package bifrost

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func validEnvironment() Environment {
	return Environment{
		APIVersion: APIVersion,
		Kind:       KindEnvironment,
		Metadata:   ObjectMeta{Name: "test-env"},
		Spec: EnvironmentSpec{
			GCP: GCPSpec{
				ProjectID:     "my-project",
				ProjectNumber: "123456",
				Region:        "us-central1",
			},
		},
	}
}

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
		},
	}
}

func TestValidate_Port(t *testing.T) {
	t.Parallel()

	env := validEnvironment()
	tests := []struct {
		name    string
		port    int32
		wantErr bool
	}{
		{"valid port", 8080, false},
		{"min valid port", 1, false},
		{"max valid port", 65535, false},
		{"zero port defaults to 8080", 0, false},
		{"negative port", -1, true},
		{"port above 65535", 65536, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := validWorkload()
			w.Spec.Port = tt.port
			err := w.Validate(env)
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

func TestValidate_PortDefaultsTo8080(t *testing.T) {
	t.Parallel()
	env := validEnvironment()
	w := validWorkload()
	w.Spec.Port = 0
	if err := w.Validate(env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Spec.Port != 8080 {
		t.Fatalf("expected port to default to 8080, got %d", w.Spec.Port)
	}
}

func TestValidate_MemoryOptional(t *testing.T) {
	t.Parallel()
	env := validEnvironment()
	w := Workload{
		APIVersion: APIVersion,
		Kind:       KindService,
		Metadata:   ObjectMeta{Name: "test-svc"},
		Spec: Spec{
			Image: "test-image",
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		},
	}
	if err := w.Validate(env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := w.Spec.Resources.Limits[corev1.ResourceMemory]; ok {
		t.Fatal("expected memory limit to be absent when not specified")
	}
}

func TestValidate_CpuStillRequired(t *testing.T) {
	t.Parallel()
	env := validEnvironment()
	w := Workload{
		APIVersion: APIVersion,
		Kind:       KindService,
		Metadata:   ObjectMeta{Name: "test-svc"},
		Spec: Spec{
			Image: "test-image",
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
		},
	}
	err := w.Validate(env)
	if err == nil {
		t.Fatal("expected error for missing cpu, got nil")
	}
}

func TestValidate_RequestsExceedLimits(t *testing.T) {
	t.Parallel()

	env := validEnvironment()
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
			err := w.Validate(env)
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

func TestValidate_TimeZoneRequired(t *testing.T) {
	t.Parallel()
	env := validEnvironment()
	w := validWorkload()
	w.Kind = KindCronJob
	w.Spec.Schedule.Cron = "0 12 * * *"
	w.Spec.Schedule.TimeZone = ""
	err := w.Validate(env)
	if err == nil {
		t.Fatal("expected error for missing timeZone, got nil")
	}
	if !strings.Contains(err.Error(), "timeZone") {
		t.Fatalf("error should mention timeZone, got: %v", err)
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

func TestValidate_CloudRunIngress(t *testing.T) {
	t.Parallel()

	env := validEnvironment()
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
			w.Spec.CloudRun.Ingress = tt.ingress
			err := w.Validate(env)
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

func TestValidate_SecretFiles(t *testing.T) {
	t.Parallel()

	env := validEnvironment()
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
			w.Spec.Schedule.TimeZone = "Etc/UTC"
			err := w.Validate(env)
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
