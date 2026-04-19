package cloudrun

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/devtools/bifrost/internal"
	"github.com/arkeros/senku/platform/secrets/gcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

func Render(spec bifrost.Workload, env bifrost.Environment) ([]byte, error) {
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
	trueValue := true
	trafficPercent := int64(100)
	concurrency := spec.Spec.Autoscaling.Concurrency
	gcp := env.Spec.GCP
	resolved := internal.ResolveSecretFiles(gcp.ProjectID, spec.Spec.SecretFiles)

	for key := range spec.Spec.SecretEnv {
		if _, ok := spec.Spec.Env[key]; ok {
			return nil, fmt.Errorf("key %q appears in both spec.env and spec.secretEnv (Cloud Run does not support env overriding secretEnv)", key)
		}
	}

	// Cloud Run rejects fractional CPU limits when containerConcurrency > 1.
	// Fail here rather than surfacing the error at `gcloud run services replace`.
	if concurrency > 1 {
		cpuLimit := spec.Spec.Resources.Limits[corev1.ResourceCPU]
		if cpuLimit.MilliValue() < 1000 {
			return nil, fmt.Errorf("spec.resources.limits.cpu must be >= 1 when spec.autoscaling.concurrency > 1 (got cpu=%s, concurrency=%d); Cloud Run rejects fractional CPU with concurrency > 1 — raise the limit or set autoscaling.concurrency=1", cpuLimit.String(), concurrency)
		}
	}

	// Cloud Run's gen2 execution environment (which bifrost always emits)
	// requires memory >= 512Mi.
	if memLimit, ok := spec.Spec.Resources.Limits[corev1.ResourceMemory]; ok {
		if memLimit.Value() < 512*1024*1024 {
			return nil, fmt.Errorf("spec.resources.limits.memory must be >= 512Mi for Cloud Run gen2 execution environment (got memory=%s)", memLimit.String())
		}
	}

	container := internal.ContainerForSpec(spec.Spec, resolved.Mounts, true, true)
	secretEnvVars, secretAliases, err := resolveSecretEnv(gcp.ProjectID, spec.Spec.SecretEnv)
	if err != nil {
		return nil, err
	}
	container.Env = append(container.Env, secretEnvVars...)

	svc := servingv1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.knative.dev/v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Metadata.Name,
			Namespace: gcp.ProjectNumber,
			Annotations: map[string]string{
				"run.googleapis.com/ingress": spec.Spec.CloudRun.Ingress,
			},
			Labels: internal.MergeStringMaps(spec.Metadata.Labels, map[string]string{
				"cloud.googleapis.com/location": gcp.Region,
			}),
		},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: internal.MergeStringMaps(templateAnnotations(gcp, spec, secretAliases), map[string]string{
							"autoscaling.knative.dev/minScale": strconv.FormatInt(int64(spec.Spec.Autoscaling.MinReplicas), 10),
							"autoscaling.knative.dev/maxScale": strconv.FormatInt(int64(spec.Spec.Autoscaling.MaxReplicas), 10),
						}),
					},
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							ServiceAccountName: spec.Spec.ServiceAccountName,
							Containers:         []corev1.Container{container},
							Volumes:            resolved.Volumes,
						},
						ContainerConcurrency: &concurrency,
					},
				},
			},
			RouteSpec: servingv1.RouteSpec{
				Traffic: []servingv1.TrafficTarget{{
					LatestRevision: &trueValue,
					Percent:        &trafficPercent,
				}},
			},
		},
	}
	if spec.Spec.CloudRun.Public {
		svc.Annotations["run.googleapis.com/invoker-iam-disabled"] = "true"
	}
	return internal.MarshalManifest(svc)
}

func renderCronJob(spec bifrost.Workload, env bifrost.Environment) ([]byte, error) {
	gcp := env.Spec.GCP
	resolved := internal.ResolveSecretFiles(gcp.ProjectID, spec.Spec.SecretFiles)

	for key := range spec.Spec.SecretEnv {
		if _, ok := spec.Spec.Env[key]; ok {
			return nil, fmt.Errorf("key %q appears in both spec.env and spec.secretEnv (Cloud Run does not support env overriding secretEnv)", key)
		}
	}

	cronContainer := internal.ContainerForSpec(spec.Spec, resolved.Mounts, false, false)
	secretEnvVars, secretAliases, err := resolveSecretEnv(gcp.ProjectID, spec.Spec.SecretEnv)
	if err != nil {
		return nil, err
	}
	cronContainer.Env = append(cronContainer.Env, secretEnvVars...)

	jobSpec := map[string]any{
		"apiVersion": "run.googleapis.com/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":      spec.Metadata.Name,
			"namespace": gcp.ProjectNumber,
			"labels": internal.MergeStringMaps(spec.Metadata.Labels, map[string]string{
				"cloud.googleapis.com/location": gcp.Region,
			}),
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"parallelism": spec.Spec.Job.Parallelism,
					"taskCount":   spec.Spec.Job.Completions,
					"template": map[string]any{
						"spec": map[string]any{
							"containers":         []corev1.Container{cronContainer},
							"maxRetries":         spec.Spec.Job.MaxRetries,
							"timeoutSeconds":     strconv.FormatInt(spec.Spec.Job.TimeoutSeconds, 10),
							"serviceAccountName": spec.Spec.ServiceAccountName,
						},
					},
				},
			},
		},
	}
	templateMetadataAnnotations := templateAnnotations(gcp, spec, secretAliases)
	if len(templateMetadataAnnotations) > 0 {
		jobSpec["spec"].(map[string]any)["template"].(map[string]any)["metadata"] = map[string]any{
			"annotations": templateMetadataAnnotations,
		}
	}
	templateSpec := jobSpec["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	if len(resolved.Volumes) > 0 {
		templateSpec["volumes"] = resolved.Volumes
	}
	return internal.MarshalManifest(jobSpec)
}

func templateAnnotations(gcpSpec bifrost.GCPSpec, spec bifrost.Workload, extraAliases []string) map[string]string {
	// Merge and deduplicate aliases from secretFiles and secretEnv.
	seen := map[string]bool{}
	var parts []string
	for _, entry := range append(strings.Split(secretsAnnotation(gcpSpec.ProjectID, spec.Spec.SecretFiles), ","), extraAliases...) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		alias, _, _ := strings.Cut(entry, ":")
		if seen[alias] {
			continue
		}
		seen[alias] = true
		parts = append(parts, entry)
	}
	return internal.MergeStringMaps(nil, map[string]string{
		"run.googleapis.com/execution-environment": "gen2",
		"run.googleapis.com/secrets":               strings.Join(parts, ","),
	})
}

func secretsAnnotation(defaultProject string, secretFiles []bifrost.SecretFile) string {
	seen := map[string]bool{}
	var parts []string
	for _, sf := range secretFiles {
		ukey := sf.UniqueKey(defaultProject)
		proj := sf.ProjectOrDefault(defaultProject)
		if proj == defaultProject || seen[ukey] {
			continue
		}
		seen[ukey] = true
		alias := sf.VolumeName(defaultProject)
		parts = append(parts, fmt.Sprintf("%s:projects/%s/secrets/%s", alias, proj, sf.Secret))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}

// resolveSecretEnv converts secretEnv URIs to Cloud Run native env vars.
// Returns env vars with valueFrom.secretKeyRef and any cross-project alias strings.
// Only plain GCP Secret Manager URIs are supported (no fragments, spreads, or transforms).
func resolveSecretEnv(defaultProject string, secretEnv map[string]string) ([]corev1.EnvVar, []string, error) {
	if len(secretEnv) == 0 {
		return nil, nil, nil
	}

	keys := make([]string, 0, len(secretEnv))
	for k := range secretEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var envVars []corev1.EnvVar
	var aliases []string
	seenAliases := map[string]bool{}

	for _, key := range keys {
		uri := secretEnv[key]

		// Reject spread keys.
		if strings.HasPrefix(key, "...") {
			return nil, nil, fmt.Errorf("secretEnv[%q]: spread is not supported on Cloud Run (use Kubernetes)", key)
		}

		// Reject transforms.
		u, err := url.Parse(uri)
		if err != nil {
			return nil, nil, fmt.Errorf("secretEnv[%q]: invalid URI: %v", key, err)
		}
		if u.Fragment != "" {
			return nil, nil, fmt.Errorf("secretEnv[%q]: JSON Pointer fragments are not supported on Cloud Run (use Kubernetes)", key)
		}
		if u.RawQuery != "" {
			return nil, nil, fmt.Errorf("secretEnv[%q]: query transforms are not supported on Cloud Run (use Kubernetes)", key)
		}

		ref, err := gcp.NewSecretRef(uri)
		if err != nil {
			return nil, nil, fmt.Errorf("secretEnv[%q]: %v (Cloud Run only supports gcp:// URIs)", key, err)
		}

		// For cross-project secrets, add alias annotation.
		secretName := ref.Name
		if ref.Project != defaultProject {
			alias := ref.Project + "--" + ref.Name
			secretName = alias
			if !seenAliases[alias] {
				seenAliases[alias] = true
				aliases = append(aliases, fmt.Sprintf("%s:projects/%s/secrets/%s", alias, ref.Project, ref.Name))
			}
		}

		envVars = append(envVars, corev1.EnvVar{
			Name: key,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  ref.Version,
				},
			},
		})
	}

	return envVars, aliases, nil
}
