package bifrost

import (
	"fmt"
	"io"

	"sigs.k8s.io/yaml"
)

const KindEnvironment = "Environment"

type GCPSpec struct {
	ProjectID     string `json:"projectId"`
	ProjectNumber string `json:"projectNumber"`
	Region        string `json:"region"`
}

type KubernetesSpec struct {
	ServiceType string `json:"serviceType,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
}

type Environment struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   ObjectMeta      `json:"metadata"`
	Spec       EnvironmentSpec `json:"spec"`
}

type EnvironmentSpec struct {
	GCP        GCPSpec         `json:"gcp"`
	Kubernetes *KubernetesSpec `json:"kubernetes,omitempty"`
}

func ParseEnvironment(r io.Reader) (Environment, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Environment{}, err
	}
	var env Environment
	if err := yaml.UnmarshalStrict(data, &env); err != nil {
		return Environment{}, err
	}
	if err := env.Validate(); err != nil {
		return Environment{}, err
	}
	return env, nil
}

func (e *Environment) Validate() error {
	if e.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", e.APIVersion)
	}
	if e.Kind != KindEnvironment {
		return fmt.Errorf("unsupported kind %q, expected %q", e.Kind, KindEnvironment)
	}
	if e.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if e.Spec.GCP.ProjectID == "" {
		return fmt.Errorf("spec.gcp.projectId is required")
	}
	if e.Spec.GCP.ProjectNumber == "" {
		return fmt.Errorf("spec.gcp.projectNumber is required")
	}
	if e.Spec.GCP.Region == "" {
		return fmt.Errorf("spec.gcp.region is required")
	}
	if e.Spec.Kubernetes != nil {
		if e.Spec.Kubernetes.ServiceType == "" {
			e.Spec.Kubernetes.ServiceType = "ClusterIP"
		}
		if e.Spec.Kubernetes.Namespace == "" {
			e.Spec.Kubernetes.Namespace = "default"
		}
	}
	return nil
}
