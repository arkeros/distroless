package k8s

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strconv"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/devtools/bifrost/internal"
	"github.com/arkeros/senku/platform/secrets/gcp"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Render(spec bifrost.Workload, env bifrost.Environment) ([]byte, error) {
	if env.Spec.Kubernetes == nil {
		return nil, fmt.Errorf("environment kubernetes is required for k8s rendering")
	}
	switch spec.Kind {
	case bifrost.KindService:
		return renderService(spec, env)
	case bifrost.KindCronJob:
		return renderCronJob(spec, env)
	default:
		return nil, fmt.Errorf("unsupported kind %q", spec.Kind)
	}
}

func renderService(spec bifrost.Workload, env bifrost.Environment) ([]byte, error) {
	labels := map[string]string{
		"app.kubernetes.io/name": spec.Metadata.Name,
	}
	gcp := env.Spec.GCP
	namespace := env.Spec.Kubernetes.Namespace
	kubernetesServiceAccountName := spec.Metadata.Name
	resolved := resolveSecretFiles(gcp.ProjectID, spec.Spec.SecretFiles)

	serviceAccount := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetesServiceAccountName,
			Namespace: namespace,
			Annotations: map[string]string{
				"iam.gke.io/gcp-service-account": spec.Spec.ServiceAccountName,
			},
		},
	}

	container := containerWithSecurityContext(withPortEnv(internal.ContainerForSpec(spec.Spec, resolved.mounts, true, true), spec.Spec.Port))
	if len(spec.Spec.SecretEnv) > 0 {
		envSecretName := spec.Metadata.Name + "-env-" + hashSecretData(spec.Spec.SecretEnv)
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: envSecretName},
			},
		})
	}

	deploy := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Metadata.Name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpecWithSecurityContext(corev1.PodSpec{
					ServiceAccountName: kubernetesServiceAccountName,
					Containers:         []corev1.Container{container},
					Volumes:            resolved.volumes,
				}),
			},
		},
	}

	minReplicas := spec.Spec.Autoscaling.MinReplicas
	if minReplicas < 1 {
		minReplicas = 1
	}
	hpa := autoscalingv2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "autoscaling/v2",
			Kind:       "HorizontalPodAutoscaler",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Metadata.Name,
			Namespace: namespace,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       spec.Metadata.Name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: spec.Spec.Autoscaling.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: int32Ptr(spec.Spec.Autoscaling.TargetCPUUtilization),
					},
				},
			}},
		},
	}

	service := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Metadata.Name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceType(env.Spec.Kubernetes.ServiceType),
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       spec.Spec.Port,
				TargetPort: intstr.FromInt32(spec.Spec.Port),
			}},
		},
	}

	serviceAccountYAML, err := internal.MarshalManifest(serviceAccount)
	if err != nil {
		return nil, err
	}
	secretsYAML, err := secretManifests(gcp.ProjectID, namespace, spec.Spec.SecretFiles)
	if err != nil {
		return nil, err
	}
	envSecretYAML, err := envSecretManifest(spec.Metadata.Name, namespace, spec.Spec.SecretEnv)
	if err != nil {
		return nil, err
	}
	deployYAML, err := internal.MarshalManifest(deploy)
	if err != nil {
		return nil, err
	}
	hpaYAML, err := internal.MarshalManifest(hpa)
	if err != nil {
		return nil, err
	}
	serviceYAML, err := internal.MarshalManifest(service)
	if err != nil {
		return nil, err
	}

	vpa := map[string]any{
		"apiVersion": "autoscaling.k8s.io/v1",
		"kind":       "VerticalPodAutoscaler",
		"metadata": map[string]any{
			"name":      spec.Metadata.Name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"targetRef": map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       spec.Metadata.Name,
			},
			"updatePolicy": map[string]any{
				"updateMode": "InPlaceOrRecreate",
			},
			"resourcePolicy": map[string]any{
				"containerPolicies": []map[string]any{{
					"containerName":       "app",
					"controlledResources": []string{"memory"},
				}},
			},
		},
	}
	vpaYAML, err := internal.MarshalManifest(vpa)
	if err != nil {
		return nil, err
	}

	var pdbYAML []byte
	if spec.Spec.Autoscaling.MinReplicas >= 2 {
		minAvailable := intstr.FromInt32(spec.Spec.Autoscaling.MinReplicas - 1)
		pdb := policyv1.PodDisruptionBudget{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "policy/v1",
				Kind:       "PodDisruptionBudget",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      spec.Metadata.Name,
				Namespace: namespace,
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &minAvailable,
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
		}
		pdbYAML, err = internal.MarshalManifest(pdb)
		if err != nil {
			return nil, err
		}
	}

	var out bytes.Buffer
	out.Write(serviceAccountYAML)
	out.WriteString("---\n")
	out.Write(secretsYAML)
	out.Write(envSecretYAML)
	out.Write(deployYAML)
	out.WriteString("---\n")
	out.Write(hpaYAML)
	out.WriteString("---\n")
	out.Write(vpaYAML)
	out.WriteString("---\n")
	if len(pdbYAML) > 0 {
		out.Write(pdbYAML)
		out.WriteString("---\n")
	}
	out.Write(serviceYAML)
	return out.Bytes(), nil
}

func renderCronJob(spec bifrost.Workload, env bifrost.Environment) ([]byte, error) {
	labels := map[string]string{
		"app.kubernetes.io/name": spec.Metadata.Name,
	}
	gcp := env.Spec.GCP
	namespace := env.Spec.Kubernetes.Namespace
	kubernetesServiceAccountName := spec.Metadata.Name
	resolved := resolveSecretFiles(gcp.ProjectID, spec.Spec.SecretFiles)

	cronContainer := containerWithSecurityContext(internal.ContainerForSpec(spec.Spec, resolved.mounts, false, false))
	if len(spec.Spec.SecretEnv) > 0 {
		envSecretName := spec.Metadata.Name + "-env-" + hashSecretData(spec.Spec.SecretEnv)
		cronContainer.EnvFrom = append(cronContainer.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: envSecretName},
			},
		})
	}

	parallelism := spec.Spec.Job.Parallelism
	completions := spec.Spec.Job.Completions
	maxRetries := spec.Spec.Job.MaxRetries
	timeoutSeconds := spec.Spec.Job.TimeoutSeconds

	serviceAccount := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetesServiceAccountName,
			Namespace: namespace,
			Annotations: map[string]string{
				"iam.gke.io/gcp-service-account": spec.Spec.ServiceAccountName,
			},
		},
	}

	cronJob := batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Metadata.Name,
			Namespace: namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: spec.Spec.Schedule.Cron,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Parallelism:           &parallelism,
					Completions:           &completions,
					BackoffLimit:          &maxRetries,
					ActiveDeadlineSeconds: &timeoutSeconds,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: podSpecWithSecurityContext(corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyNever,
							ServiceAccountName: kubernetesServiceAccountName,
							Containers:         []corev1.Container{cronContainer},
							Volumes:            resolved.volumes,
						}),
					},
				},
			},
		},
	}
	if spec.Spec.Schedule.TimeZone != "" {
		cronJob.Spec.TimeZone = stringPtr(spec.Spec.Schedule.TimeZone)
	}

	serviceAccountYAML, err := internal.MarshalManifest(serviceAccount)
	if err != nil {
		return nil, err
	}
	secretsYAML, err := secretManifests(gcp.ProjectID, namespace, spec.Spec.SecretFiles)
	if err != nil {
		return nil, err
	}
	envSecretYAML, err := envSecretManifest(spec.Metadata.Name, namespace, spec.Spec.SecretEnv)
	if err != nil {
		return nil, err
	}
	cronJobYAML, err := internal.MarshalManifest(cronJob)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.Write(serviceAccountYAML)
	out.WriteString("---\n")
	out.Write(secretsYAML)
	out.Write(envSecretYAML)
	out.Write(cronJobYAML)
	return out.Bytes(), nil
}

// resolveSecretFiles expands secretFiles into hashed k8s Volumes and VolumeMounts.
// Names are suffixed with a content hash so that changes trigger pod rollouts.
type resolvedSecrets struct {
	volumes []corev1.Volume
	mounts  []corev1.VolumeMount
}

func resolveSecretFiles(projectID string, secretFiles []bifrost.SecretFile) resolvedSecrets {
	if len(secretFiles) == 0 {
		return resolvedSecrets{}
	}
	type secretData struct {
		name string
		data map[string]string
	}
	type mountGroup struct {
		ukey      string
		mountPath string
	}
	secrets := map[string]*secretData{}
	var secretOrder []string
	groups := map[string]*mountGroup{}
	var groupOrder []string
	for _, sf := range secretFiles {
		ukey := sf.UniqueKey(projectID)
		proj := sf.ProjectOrDefault(projectID)
		version := sf.VersionString()
		basename := path.Base(sf.Path)
		vname := sf.VolumeName(projectID)
		sd, ok := secrets[ukey]
		if !ok {
			sd = &secretData{name: vname, data: map[string]string{}}
			secrets[ukey] = sd
			secretOrder = append(secretOrder, ukey)
		}
		sd.data[basename] = gcp.URI(proj, sf.Secret, version)

		dir := path.Dir(sf.Path)
		gkey := ukey + ":" + dir
		if _, ok := groups[gkey]; !ok {
			groups[gkey] = &mountGroup{ukey: ukey, mountPath: dir}
			groupOrder = append(groupOrder, gkey)
		}
	}
	suffixedNames := map[string]string{}
	for _, ukey := range secretOrder {
		sd := secrets[ukey]
		suffixedNames[ukey] = sd.name + "-" + hashSecretData(sd.data)
	}
	var res resolvedSecrets
	for _, gkey := range groupOrder {
		g := groups[gkey]
		sname := suffixedNames[g.ukey]
		res.volumes = append(res.volumes, corev1.Volume{
			Name: sname,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: sname,
				},
			},
		})
		res.mounts = append(res.mounts, corev1.VolumeMount{
			Name:      sname,
			MountPath: g.mountPath,
		})
	}
	return res
}

func secretManifests(projectID, namespace string, secretFiles []bifrost.SecretFile) ([]byte, error) {
	if len(secretFiles) == 0 {
		return nil, nil
	}
	type secretEntry struct {
		name string
		data map[string]string
	}
	seen := map[string]*secretEntry{}
	var order []string
	for _, sf := range secretFiles {
		ukey := sf.UniqueKey(projectID)
		proj := sf.ProjectOrDefault(projectID)
		version := sf.VersionString()
		basename := path.Base(sf.Path)
		vname := sf.VolumeName(projectID)
		e, ok := seen[ukey]
		if !ok {
			e = &secretEntry{name: vname, data: map[string]string{}}
			seen[ukey] = e
			order = append(order, ukey)
		}
		e.data[basename] = gcp.URI(proj, sf.Secret, version)
	}
	var out bytes.Buffer
	for _, ukey := range order {
		e := seen[ukey]
		suffixed := e.name + "-" + hashSecretData(e.data)
		secret := map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      suffixed,
				"namespace": namespace,
			},
			"stringData": e.data,
		}
		b, err := internal.MarshalManifest(secret)
		if err != nil {
			return nil, err
		}
		out.Write(b)
		out.WriteString("---\n")
	}
	return out.Bytes(), nil
}

func envSecretManifest(workloadName, namespace string, secretEnv map[string]string) ([]byte, error) {
	if len(secretEnv) == 0 {
		return nil, nil
	}
	suffixed := workloadName + "-env-" + hashSecretData(secretEnv)
	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      suffixed,
			"namespace": namespace,
		},
		"stringData": secretEnv,
	}
	b, err := internal.MarshalManifest(secret)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Write(b)
	out.WriteString("---\n")
	return out.Bytes(), nil
}

func hashSecretData(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(data[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

func containerWithSecurityContext(c corev1.Container) corev1.Container {
	trueVal := true
	falseVal := false
	c.SecurityContext = &corev1.SecurityContext{
		RunAsNonRoot:             &trueVal,
		AllowPrivilegeEscalation: &falseVal,
		ReadOnlyRootFilesystem:   &trueVal,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	return c
}

func int32Ptr(v int32) *int32 {
	return &v
}

func stringPtr(v string) *string {
	return &v
}

func withPortEnv(c corev1.Container, port int32) corev1.Container {
	if port > 0 {
		c.Env = append(c.Env, corev1.EnvVar{
			Name:  "PORT",
			Value: strconv.FormatInt(int64(port), 10),
		})
	}
	return c
}

func podSpecWithSecurityContext(spec corev1.PodSpec) corev1.PodSpec {
	trueVal := true
	spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot: &trueVal,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	return spec
}
