package internal

import (
	"path"
	"sort"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
)

type ResolvedSecrets struct {
	Volumes []corev1.Volume
	Mounts  []corev1.VolumeMount
}

func ResolveSecretFiles(projectID string, secretFiles []bifrost.SecretFile) ResolvedSecrets {
	if len(secretFiles) == 0 {
		return ResolvedSecrets{}
	}
	type mountGroup struct {
		name      string
		mountPath string
		items     []corev1.KeyToPath
	}
	groups := map[string]*mountGroup{}
	var order []string
	for _, sf := range secretFiles {
		version := sf.VersionString()
		dir := path.Dir(sf.Path)
		ukey := sf.UniqueKey(projectID)
		gkey := ukey + ":" + dir
		g, ok := groups[gkey]
		if !ok {
			g = &mountGroup{name: sf.Secret, mountPath: dir}
			groups[gkey] = g
			order = append(order, gkey)
		}
		g.items = append(g.items, corev1.KeyToPath{
			Key:  version,
			Path: path.Base(sf.Path),
		})
	}
	var res ResolvedSecrets
	for _, gkey := range order {
		g := groups[gkey]
		res.Volumes = append(res.Volumes, corev1.Volume{
			Name: g.name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: g.name,
					Items:      g.items,
				},
			},
		})
		res.Mounts = append(res.Mounts, corev1.VolumeMount{
			Name:      g.name,
			MountPath: g.mountPath,
		})
	}
	return res
}

func ContainerForSpec(spec bifrost.Spec, volumeMounts []corev1.VolumeMount, includePorts bool, includeProbes bool) corev1.Container {
	resources := *spec.Resources.DeepCopy()
	container := corev1.Container{
		Name:         "app",
		Image:        spec.Image,
		Args:         SlicesClone(spec.Args),
		Env:          envVarsFromMap(spec.Env),
		Resources:    resources,
		VolumeMounts: volumeMounts,
	}
	if includePorts && spec.Port > 0 {
		container.Ports = []corev1.ContainerPort{{
			ContainerPort: spec.Port,
		}}
	}
	if includeProbes {
		container.StartupProbe = httpGetProbe(spec.Probes.StartupPath, spec.Port)
		container.LivenessProbe = httpGetProbe(spec.Probes.LivenessPath, spec.Port)
	}
	return container
}

func envVarsFromMap(env map[string]string) []corev1.EnvVar {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	vars := make([]corev1.EnvVar, len(keys))
	for i, k := range keys {
		vars[i] = corev1.EnvVar{Name: k, Value: env[k]}
	}
	return vars
}

func httpGetProbe(path string, port int32) *corev1.Probe {
	if path == "" {
		return nil
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt32(port),
			},
		},
	}
}

func MarshalManifest(obj any) ([]byte, error) {
	raw, err := yaml.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var manifest map[string]any
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	delete(manifest, "status")
	return yaml.Marshal(manifest)
}

func MergeStringMaps(base map[string]string, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range base {
		if v != "" {
			out[k] = v
		}
	}
	for k, v := range extra {
		if v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func SlicesClone(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
