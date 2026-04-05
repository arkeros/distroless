package cloudrun

import (
	"fmt"
	"strconv"
	"strings"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/arkeros/senku/devtools/bifrost/internal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

func Render(spec bifrost.Workload) ([]byte, error) {
	switch spec.Kind {
	case bifrost.KindService:
		return renderService(spec)
	case bifrost.KindCronJob:
		return renderCronJob(spec)
	default:
		return nil, fmt.Errorf("unsupported kind %q", spec.Kind)
	}
}

func renderService(spec bifrost.Workload) ([]byte, error) {
	trueValue := true
	trafficPercent := int64(100)
	concurrency := spec.Spec.Autoscaling.Concurrency
	resolved := internal.ResolveSecretFiles(spec.Spec.GCP.ProjectID, spec.Spec.SecretFiles)

	svc := servingv1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.knative.dev/v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Metadata.Name,
			Namespace: spec.Spec.GCP.ProjectNumber,
			Annotations: map[string]string{
				"run.googleapis.com/ingress": spec.Spec.GCP.CloudRun.Ingress,
			},
			Labels: internal.MergeStringMaps(spec.Metadata.Labels, map[string]string{
				"cloud.googleapis.com/location": spec.Spec.GCP.Region,
			}),
		},
		Spec: servingv1.ServiceSpec{
			ConfigurationSpec: servingv1.ConfigurationSpec{
				Template: servingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: internal.MergeStringMaps(templateAnnotations(spec), map[string]string{
							"autoscaling.knative.dev/minScale": strconv.FormatInt(int64(spec.Spec.Autoscaling.MinReplicas), 10),
							"autoscaling.knative.dev/maxScale": strconv.FormatInt(int64(spec.Spec.Autoscaling.MaxReplicas), 10),
						}),
					},
					Spec: servingv1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							ServiceAccountName: spec.Spec.ServiceAccountName,
							Containers:         []corev1.Container{internal.ContainerForSpec(spec.Spec, resolved.Mounts, true, true)},
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
	if spec.Spec.GCP.CloudRun.Public {
		svc.Annotations["run.googleapis.com/invoker-iam-disabled"] = "true"
	}
	return internal.MarshalManifest(svc)
}

func renderCronJob(spec bifrost.Workload) ([]byte, error) {
	resolved := internal.ResolveSecretFiles(spec.Spec.GCP.ProjectID, spec.Spec.SecretFiles)
	jobSpec := map[string]any{
		"apiVersion": "run.googleapis.com/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":      spec.Metadata.Name,
			"namespace": spec.Spec.GCP.ProjectNumber,
			"labels": internal.MergeStringMaps(spec.Metadata.Labels, map[string]string{
				"cloud.googleapis.com/location": spec.Spec.GCP.Region,
			}),
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"parallelism": spec.Spec.Job.Parallelism,
					"taskCount":   spec.Spec.Job.Completions,
					"template": map[string]any{
						"spec": map[string]any{
							"containers":         []corev1.Container{internal.ContainerForSpec(spec.Spec, resolved.Mounts, false, false)},
							"maxRetries":         spec.Spec.Job.MaxRetries,
							"timeoutSeconds":     strconv.FormatInt(spec.Spec.Job.TimeoutSeconds, 10),
							"serviceAccountName": spec.Spec.ServiceAccountName,
						},
					},
				},
			},
		},
	}
	templateMetadataAnnotations := templateAnnotations(spec)
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

func templateAnnotations(spec bifrost.Workload) map[string]string {
	return internal.MergeStringMaps(nil, map[string]string{
		"run.googleapis.com/execution-environment": "gen2",
		"run.googleapis.com/secrets":               secretsAnnotation(spec.Spec.GCP.ProjectID, spec.Spec.SecretFiles),
	})
}

func secretsAnnotation(defaultProject string, secretFiles []bifrost.SecretFile) string {
	seen := map[string]bool{}
	var parts []string
	for _, sf := range secretFiles {
		ukey := sf.UniqueKey(defaultProject)
		proj, name, _ := sf.ParseSecret(defaultProject)
		if proj == defaultProject || seen[ukey] {
			continue
		}
		seen[ukey] = true
		parts = append(parts, fmt.Sprintf("%s:projects/%s/secrets/%s", name, proj, name))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}
