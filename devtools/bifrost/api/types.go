package bifrost

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

const (
	APIVersion  = "bifrost.apotema.cloud/v1alpha1"
	KindService = "Service"
	KindCronJob = "CronJob"
)

type Workload struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       Spec       `json:"spec"`
}

type Service = Workload

type ObjectMeta struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

type SecretFile struct {
	Secret string `json:"secret"`
	Path   string `json:"path"`
}

// ParseSecret parses the Secret field into project, name, and version components.
// Accepted formats:
//   - "name"                              → (defaultProject, "name", "latest")
//   - "projects/P/secrets/name"           → ("P", "name", "latest")
//   - "projects/P/secrets/name/versions/V" → ("P", "name", "V")
func (sf SecretFile) ParseSecret(defaultProject string) (project, name, version string) {
	s := sf.Secret
	if !strings.HasPrefix(s, "projects/") {
		return defaultProject, s, "latest"
	}
	s = strings.TrimPrefix(s, "projects/")
	project, s, ok := strings.Cut(s, "/secrets/")
	if !ok || project == "" || s == "" {
		return defaultProject, sf.Secret, "latest"
	}
	name, version, ok = strings.Cut(s, "/versions/")
	if !ok || version == "" {
		version = "latest"
	}
	return project, name, version
}

// UniqueKey returns a key that uniquely identifies this secret across projects.
func (sf SecretFile) UniqueKey(defaultProject string) string {
	project, name, _ := sf.ParseSecret(defaultProject)
	return project + "/" + name
}

func validateSecretFiles(secretFiles []SecretFile) error {
	for i, sf := range secretFiles {
		if sf.Secret == "" {
			return fmt.Errorf("spec.secretFiles[%d].secret is required", i)
		}
		if sf.Path == "" {
			return fmt.Errorf("spec.secretFiles[%d].path is required", i)
		}
		if !strings.HasPrefix(sf.Path, "/") {
			return fmt.Errorf("spec.secretFiles[%d].path must be absolute", i)
		}
		if strings.HasPrefix(sf.Secret, "projects/") {
			s := strings.TrimPrefix(sf.Secret, "projects/")
			project, rest, ok := strings.Cut(s, "/secrets/")
			if !ok || project == "" || rest == "" {
				return fmt.Errorf("spec.secretFiles[%d].secret %q is not a valid GCP Secret Manager resource path", i, sf.Secret)
			}
		}
	}
	return nil
}

type Spec struct {
	Image              string                      `json:"image"`
	ServiceAccountName string                      `json:"serviceAccountName,omitempty"`
	Args               []string                    `json:"args,omitempty"`
	Port               int32                       `json:"port,omitempty"`
	Env                map[string]string            `json:"env,omitempty"`
	Resources          corev1.ResourceRequirements `json:"resources"`
	SecretFiles        []SecretFile                `json:"secretFiles,omitempty"`
	Probes             ProbeSpec                   `json:"probes,omitempty"`
	Autoscaling        AutoscalingSpec             `json:"autoscaling,omitempty"`
	Schedule           ScheduleSpec                `json:"schedule,omitempty"`
	Job                JobSpec                     `json:"job,omitempty"`
	CloudRun           CloudRunSpec                `json:"cloudRun,omitempty"`
	CloudScheduler     CloudSchedulerSpec          `json:"cloudScheduler,omitempty"`
}

type CloudRunSpec struct {
	Ingress string `json:"ingress,omitempty"`
	Public  bool   `json:"public,omitempty"`
}

type CloudSchedulerSpec struct {
	RetryCount             int32 `json:"retryCount,omitempty"`
	AttemptDeadlineSeconds int64 `json:"attemptDeadlineSeconds,omitempty"`
}

type ProbeSpec struct {
	StartupPath  string `json:"startupPath,omitempty"`
	LivenessPath string `json:"livenessPath,omitempty"`
}

type ScheduleSpec struct {
	Cron     string `json:"cron,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type JobSpec struct {
	Parallelism    int32 `json:"parallelism,omitempty"`
	Completions    int32 `json:"completions,omitempty"`
	MaxRetries     int32 `json:"maxRetries,omitempty"`
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`
}

type AutoscalingSpec struct {
	MinReplicas          int32 `json:"min,omitempty"`
	MaxReplicas          int32 `json:"max,omitempty"`
	Concurrency          int64 `json:"concurrency,omitempty"`
	TargetCPUUtilization int32 `json:"targetCPUUtilization,omitempty"`
}

func Parse(r io.Reader, env Environment) (Workload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Workload{}, err
	}
	var spec Workload
	if err := yaml.UnmarshalStrict(data, &spec); err != nil {
		return Workload{}, err
	}
	if err := spec.Validate(env); err != nil {
		return Workload{}, err
	}
	return spec, nil
}

func (s *Workload) Validate(env Environment) error {
	if s.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", s.APIVersion)
	}
	if s.Kind != KindService && s.Kind != KindCronJob {
		return fmt.Errorf("unsupported kind %q", s.Kind)
	}
	if s.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if s.Spec.Image == "" {
		return fmt.Errorf("spec.image is required")
	}
	projectID := env.Spec.GCP.ProjectID
	if s.Spec.ServiceAccountName == "" {
		prefix := "svc-"
		if s.Kind == KindCronJob {
			prefix = "crj-"
		}
		accountID, err := DefaultServiceAccountAccountID(prefix, s.Metadata.Name)
		if err != nil {
			return err
		}
		s.Spec.ServiceAccountName = fmt.Sprintf("%s@%s.iam.gserviceaccount.com", accountID, projectID)
	}
	if !strings.Contains(s.Spec.ServiceAccountName, "@") {
		return fmt.Errorf("spec.serviceAccountName must be a Google service account email")
	}
	if saProjectID, err := projectIDFromServiceAccountEmail(s.Spec.ServiceAccountName); err != nil {
		return err
	} else if saProjectID != projectID {
		return fmt.Errorf("spec.serviceAccountName project %q does not match environment gcp.projectId %q", saProjectID, projectID)
	}
	if s.Kind == KindService && s.Spec.Port == 0 {
		s.Spec.Port = 8080
	}
	if s.Kind == KindService && (s.Spec.Port < 1 || s.Spec.Port > 65535) {
		return fmt.Errorf("spec.port must be between 1 and 65535")
	}
	if s.Spec.Resources.Limits == nil {
		s.Spec.Resources.Limits = corev1.ResourceList{}
	}
	if s.Spec.Resources.Requests == nil {
		s.Spec.Resources.Requests = corev1.ResourceList{}
	}
	if _, ok := s.Spec.Resources.Limits[corev1.ResourceCPU]; !ok {
		return fmt.Errorf("spec.resources.limits.cpu is required")
	}
	if _, ok := s.Spec.Resources.Requests[corev1.ResourceCPU]; !ok {
		s.Spec.Resources.Requests[corev1.ResourceCPU] = s.Spec.Resources.Limits[corev1.ResourceCPU]
	}
	if err := validateResourceQuantity("spec.resources.requests.cpu", s.Spec.Resources.Requests[corev1.ResourceCPU]); err != nil {
		return err
	}
	if err := validateResourceQuantity("spec.resources.limits.cpu", s.Spec.Resources.Limits[corev1.ResourceCPU]); err != nil {
		return err
	}
	if reqCPU := s.Spec.Resources.Requests[corev1.ResourceCPU]; reqCPU.Cmp(s.Spec.Resources.Limits[corev1.ResourceCPU]) > 0 {
		return fmt.Errorf("spec.resources.requests.cpu must be <= spec.resources.limits.cpu")
	}
	if memLimit, ok := s.Spec.Resources.Limits[corev1.ResourceMemory]; ok {
		if err := validateResourceQuantity("spec.resources.limits.memory", memLimit); err != nil {
			return err
		}
		if _, ok := s.Spec.Resources.Requests[corev1.ResourceMemory]; !ok {
			s.Spec.Resources.Requests[corev1.ResourceMemory] = memLimit
		}
		if err := validateResourceQuantity("spec.resources.requests.memory", s.Spec.Resources.Requests[corev1.ResourceMemory]); err != nil {
			return err
		}
		if reqMem := s.Spec.Resources.Requests[corev1.ResourceMemory]; reqMem.Cmp(memLimit) > 0 {
			return fmt.Errorf("spec.resources.requests.memory must be <= spec.resources.limits.memory")
		}
	}
	if err := validateSecretFiles(s.Spec.SecretFiles); err != nil {
		return err
	}
	if s.Spec.CloudRun.Ingress == "" {
		s.Spec.CloudRun.Ingress = "all"
	}
	switch s.Spec.CloudRun.Ingress {
	case "all", "internal", "internal-and-cloud-load-balancing":
	default:
		return fmt.Errorf("spec.cloudRun.ingress %q is not valid, must be one of: all, internal, internal-and-cloud-load-balancing", s.Spec.CloudRun.Ingress)
	}
	if s.Spec.Autoscaling.MinReplicas < 0 {
		return fmt.Errorf("spec.autoscaling.min must be greater than or equal to zero")
	}
	if s.Spec.Autoscaling.MaxReplicas <= 0 {
		s.Spec.Autoscaling.MaxReplicas = 3
	}
	if s.Spec.Autoscaling.MaxReplicas < s.Spec.Autoscaling.MinReplicas {
		return fmt.Errorf("spec.autoscaling.max must be greater than or equal to min")
	}
	if s.Spec.Autoscaling.Concurrency <= 0 {
		s.Spec.Autoscaling.Concurrency = 80
	}
	if s.Spec.Autoscaling.TargetCPUUtilization <= 0 {
		s.Spec.Autoscaling.TargetCPUUtilization = 80
	}
	if s.Kind == KindCronJob {
		if strings.TrimSpace(s.Spec.Schedule.Cron) == "" {
			return fmt.Errorf("spec.schedule.cron is required for kind %q", KindCronJob)
		}
		if strings.TrimSpace(s.Spec.Schedule.TimeZone) == "" {
			return fmt.Errorf("spec.schedule.timeZone is required for kind %q", KindCronJob)
		}
		if s.Spec.Job.Parallelism <= 0 {
			s.Spec.Job.Parallelism = 1
		}
		if s.Spec.Job.Completions <= 0 {
			s.Spec.Job.Completions = 1
		}
		if s.Spec.Job.MaxRetries < 0 {
			return fmt.Errorf("spec.job.maxRetries must be greater than or equal to zero")
		}
		if s.Spec.Job.MaxRetries == 0 {
			s.Spec.Job.MaxRetries = 3
		}
		if s.Spec.Job.TimeoutSeconds <= 0 {
			s.Spec.Job.TimeoutSeconds = 600
		}
	}
	return nil
}

func validateResourceQuantity(field string, q resource.Quantity) error {
	if q.Sign() <= 0 {
		return fmt.Errorf("%s must be greater than zero", field)
	}
	return nil
}

func projectIDFromServiceAccountEmail(email string) (string, error) {
	local, domain, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" || domain == "" {
		return "", fmt.Errorf("invalid service account email %q", email)
	}
	const suffix = ".iam.gserviceaccount.com"
	if !strings.HasSuffix(domain, suffix) {
		return "", fmt.Errorf("spec.serviceAccountName must be a Google service account email")
	}
	return strings.TrimSuffix(domain, suffix), nil
}

func DefaultServiceAccountAccountID(prefix, name string) (string, error) {
	var out strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(unicode.ToLower(r))
		case r == '-', r == '.':
			out.WriteByte('-')
		}
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("name %q does not produce a valid default service account", name)
	}
	accountID := prefix + strings.Trim(out.String(), "-")
	if len(accountID) < 6 || len(accountID) > 30 {
		return "", fmt.Errorf("default service account account ID %q must be between 6 and 30 characters", accountID)
	}
	return accountID, nil
}
