package kubernetes

import (
	"fmt"
	"io"
	pathpkg "path"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	MaxPodTemplateBytes    = 256 * 1024
	DefaultAgentctlPort    = int32(8765)
	RuntimeVolumeName      = "kandev-runtime"
	AuthVolumeName         = "kandev-auth"
	WorkspaceVolumeName    = "kandev-workspace"
	RuntimeMountPath       = "/opt/kandev"
	AuthMountPath          = "/run/kandev"
	WorkspaceMountPath     = "/workspace"
	AgentctlContainerPort  = "kandev-agentctl"
	podTemplateFieldPrefix = "config.pod_template_yaml"
)

var reservedLabelKeys = map[string]bool{
	"app.kubernetes.io/component":  true,
	"app.kubernetes.io/instance":   true,
	"app.kubernetes.io/managed-by": true,
	"app.kubernetes.io/name":       true,
}

// ParsePodTemplate decodes exactly one strict core/v1 PodTemplate document.
func ParsePodTemplate(raw string) (*corev1.PodTemplate, error) {
	if len(raw) > MaxPodTemplateBytes {
		return nil, fieldError(podTemplateFieldPrefix, "must not exceed 256 KiB")
	}
	if strings.TrimSpace(raw) == "" {
		return nil, fieldError(podTemplateFieldPrefix, "is required")
	}
	if err := requireSingleYAMLDocument(raw); err != nil {
		return nil, err
	}
	var template corev1.PodTemplate
	if err := sigsyaml.UnmarshalStrict([]byte(raw), &template); err != nil {
		return nil, fieldError(podTemplateFieldPrefix, "must be strict PodTemplate YAML")
	}
	if template.APIVersion != "v1" || template.Kind != "PodTemplate" {
		return nil, fieldError(podTemplateFieldPrefix, "must have apiVersion v1 and kind PodTemplate")
	}
	return &template, nil
}

// ValidatePodTemplate rejects fields owned by Kandev before any API write.
func ValidatePodTemplate(template *corev1.PodTemplate, mainContainer string) error {
	if template == nil {
		return fieldError(podTemplateFieldPrefix, "is required")
	}
	if err := validateTemplateMetadata(template); err != nil {
		return err
	}
	if err := validatePodSpecFields(&template.Template.Spec); err != nil {
		return err
	}
	return validateTemplateContainers(&template.Template.Spec, mainContainer)
}

func requireSingleYAMLDocument(raw string) error {
	decoder := yamlv3.NewDecoder(strings.NewReader(raw))
	var document yamlv3.Node
	if err := decoder.Decode(&document); err != nil {
		return fieldError(podTemplateFieldPrefix, "must be valid YAML")
	}
	var extra yamlv3.Node
	err := decoder.Decode(&extra)
	if err == nil {
		return fieldError(podTemplateFieldPrefix, "must contain exactly one YAML document")
	}
	if err != io.EOF {
		return fieldError(podTemplateFieldPrefix, "must be valid YAML")
	}
	return nil
}

func validateTemplateMetadata(template *corev1.PodTemplate) error {
	meta := &template.Template.ObjectMeta
	checks := []struct {
		set  bool
		path string
	}{
		{meta.Name != "", ".template.metadata.name"},
		{meta.GenerateName != "", ".template.metadata.generateName"},
		{meta.Namespace != "", ".template.metadata.namespace"},
		{meta.UID != "", ".template.metadata.uid"},
		{meta.ResourceVersion != "", ".template.metadata.resourceVersion"},
		{meta.Generation != 0, ".template.metadata.generation"},
		{!meta.CreationTimestamp.IsZero(), ".template.metadata.creationTimestamp"},
		{meta.DeletionTimestamp != nil, ".template.metadata.deletionTimestamp"},
		{len(meta.OwnerReferences) > 0, ".template.metadata.ownerReferences"},
		{len(meta.Finalizers) > 0, ".template.metadata.finalizers"},
		{len(meta.ManagedFields) > 0, ".template.metadata.managedFields"},
	}
	for _, check := range checks {
		if check.set {
			return reservedField(check.path)
		}
	}
	for key := range meta.Labels {
		if reservedLabelKeys[key] || strings.HasPrefix(key, "kandev.ai/") {
			return reservedField(".template.metadata.labels." + key)
		}
	}
	for key := range meta.Annotations {
		if strings.HasPrefix(key, "kandev.ai/") {
			return reservedField(".template.metadata.annotations." + key)
		}
	}
	return nil
}

func validatePodSpecFields(spec *corev1.PodSpec) error {
	if spec.RestartPolicy != "" {
		return reservedField(".template.spec.restartPolicy")
	}
	if spec.NodeName != "" {
		return reservedField(".template.spec.nodeName")
	}
	if spec.OS != nil {
		return reservedField(".template.spec.os")
	}
	for _, key := range []string{corev1.LabelOSStable, corev1.LabelArchStable} {
		if _, ok := spec.NodeSelector[key]; ok {
			return reservedField(".template.spec.nodeSelector." + key)
		}
	}
	if err := validateNodeAffinity(spec.Affinity); err != nil {
		return err
	}
	for i, volume := range spec.Volumes {
		if isReservedVolumeName(volume.Name) {
			return reservedField(fmt.Sprintf(".template.spec.volumes[%d].name", i))
		}
	}
	return nil
}

func validateTemplateContainers(spec *corev1.PodSpec, mainContainer string) error {
	mainIndex := -1
	for i := range spec.Containers {
		container := &spec.Containers[i]
		path := fmt.Sprintf(".template.spec.containers[%d]", i)
		if container.Image == "" {
			return fieldError(podTemplateFieldPrefix+path+".image", "is required")
		}
		if container.Name == mainContainer {
			if mainIndex >= 0 {
				return fieldError(podTemplateFieldPrefix+path+".name", "duplicates the main container")
			}
			mainIndex = i
			if err := validateMainContainerFields(container, path); err != nil {
				return err
			}
		}
		if err := validateContainerReservedFields(container, path); err != nil {
			return err
		}
	}
	if mainIndex < 0 {
		return fieldError(podTemplateFieldPrefix+".template.spec.containers", "must contain the configured main container")
	}
	return validateAuxiliaryContainers(spec)
}

func validateMainContainerFields(container *corev1.Container, path string) error {
	switch {
	case len(container.Command) > 0:
		return reservedField(path + ".command")
	case len(container.Args) > 0:
		return reservedField(path + ".args")
	case container.WorkingDir != "":
		return reservedField(path + ".workingDir")
	default:
		return nil
	}
}

func validateContainerReservedFields(container *corev1.Container, path string) error {
	if len(container.EnvFrom) > 0 {
		return reservedField(path + ".envFrom")
	}
	for i, env := range container.Env {
		if isReservedEnvironmentKey(env.Name) {
			return reservedField(fmt.Sprintf("%s.env[%d].name", path, i))
		}
	}
	for i, port := range container.Ports {
		if port.Name == AgentctlContainerPort || port.ContainerPort == DefaultAgentctlPort {
			return reservedField(fmt.Sprintf("%s.ports[%d]", path, i))
		}
	}
	for i, mount := range container.VolumeMounts {
		if isReservedVolumeName(mount.Name) || isReservedMountPath(mount.MountPath) {
			return reservedField(fmt.Sprintf("%s.volumeMounts[%d]", path, i))
		}
	}
	for i, device := range container.VolumeDevices {
		if isReservedVolumeName(device.Name) {
			return reservedField(fmt.Sprintf("%s.volumeDevices[%d].name", path, i))
		}
		if isReservedMountPath(device.DevicePath) {
			return reservedField(fmt.Sprintf("%s.volumeDevices[%d].devicePath", path, i))
		}
	}
	return nil
}

func isReservedEnvironmentKey(name string) bool {
	return name == "HOME" || strings.HasPrefix(name, "AGENTCTL_") || strings.HasPrefix(name, "KANDEV_")
}

func validateAuxiliaryContainers(spec *corev1.PodSpec) error {
	for i := range spec.InitContainers {
		path := fmt.Sprintf(".template.spec.initContainers[%d]", i)
		if err := validateContainerReservedFields(&spec.InitContainers[i], path); err != nil {
			return err
		}
	}
	for i := range spec.EphemeralContainers {
		path := fmt.Sprintf(".template.spec.ephemeralContainers[%d]", i)
		container := corev1.Container(spec.EphemeralContainers[i].EphemeralContainerCommon)
		if err := validateContainerReservedFields(&container, path); err != nil {
			return err
		}
	}
	return nil
}

func validateNodeAffinity(affinity *corev1.Affinity) error {
	if affinity == nil || affinity.NodeAffinity == nil {
		return nil
	}
	nodeAffinity := affinity.NodeAffinity
	if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		for i, term := range nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			if path := reservedSelectorPath(term, fmt.Sprintf(".template.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[%d]", i)); path != "" {
				return reservedField(path)
			}
		}
	}
	for i, term := range nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
		base := fmt.Sprintf(".template.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[%d].preference", i)
		if path := reservedSelectorPath(term.Preference, base); path != "" {
			return reservedField(path)
		}
	}
	return nil
}

func reservedSelectorPath(term corev1.NodeSelectorTerm, base string) string {
	for i, requirement := range term.MatchExpressions {
		if requirement.Key == corev1.LabelOSStable || requirement.Key == corev1.LabelArchStable {
			return fmt.Sprintf("%s.matchExpressions[%d].key", base, i)
		}
	}
	for i, requirement := range term.MatchFields {
		if requirement.Key == corev1.LabelOSStable || requirement.Key == corev1.LabelArchStable {
			return fmt.Sprintf("%s.matchFields[%d].key", base, i)
		}
	}
	return ""
}

func isReservedVolumeName(name string) bool {
	return name == RuntimeVolumeName || name == AuthVolumeName || name == WorkspaceVolumeName
}

func isReservedMountPath(path string) bool {
	cleaned := pathpkg.Clean(path)
	return isPathAtOrBelow(cleaned, RuntimeMountPath) ||
		isPathAtOrBelow(cleaned, AuthMountPath) ||
		isPathAtOrBelow(cleaned, WorkspaceMountPath)
}

func isPathAtOrBelow(candidate, root string) bool {
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

func reservedField(suffix string) error {
	return fieldError(podTemplateFieldPrefix+suffix, "is reserved for Kandev")
}
