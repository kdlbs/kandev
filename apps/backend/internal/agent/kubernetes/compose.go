package kubernetes

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Runtime inventory metadata keys are shared by lifecycle persistence and
// higher-level fail-closed inventory checks.
const (
	MetadataKeyResourceExecutorID    = "kubernetes_resource_executor_id"
	MetadataKeyResourceProfileID     = "kubernetes_resource_profile_id"
	MetadataKeyResourceInstanceID    = "kubernetes_resource_instance_id"
	MetadataKeyResourceTaskID        = "kubernetes_resource_task_id"
	MetadataKeyResourceSessionID     = "kubernetes_resource_session_id"
	MetadataKeyResourceEnvironmentID = "kubernetes_resource_environment_id"
)

type ResourceIdentity struct {
	ExecutorID    string
	ProfileID     string
	InstanceID    string
	TaskID        string
	SessionID     string
	EnvironmentID string
}

type PodOptions struct {
	Name           string
	Namespace      string
	Identity       ResourceIdentity
	Command        []string
	Args           []string
	WorkingDir     string
	AgentctlPort   int32
	ManagedPVCName string
}

type Warning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func ComposePod(template *corev1.PodTemplate, profile ProfileConfig, options PodOptions) (*corev1.Pod, []Warning, error) {
	if err := validateComposition(template, profile, options); err != nil {
		return nil, nil, err
	}
	labels, err := OwnershipLabels(options.Identity)
	if err != nil {
		return nil, nil, err
	}
	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: *template.Template.ObjectMeta.DeepCopy(),
		Spec:       *template.Template.Spec.DeepCopy(),
	}
	pod.Name = options.Name
	pod.Namespace = options.Namespace
	if pod.Labels == nil {
		pod.Labels = make(map[string]string, len(labels))
	}
	for key, value := range labels {
		pod.Labels[key] = value
	}
	applyPodInvariants(pod, profile.Platform)
	applyMainContainer(pod, profile.MainContainer, options)
	workspace, err := workspaceVolume(profile.Workspace, options.ManagedPVCName)
	if err != nil {
		return nil, nil, err
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, managedVolumes(workspace)...)
	return pod, collectWarnings(pod), nil
}

func OwnershipLabels(identity ResourceIdentity) (map[string]string, error) {
	values := []struct {
		path  string
		key   string
		value string
	}{
		{"identity.executor_id", "kandev.ai/executor-id", identity.ExecutorID},
		{"identity.profile_id", "kandev.ai/profile-id", identity.ProfileID},
		{"identity.instance_id", "kandev.ai/instance-id", identity.InstanceID},
		{"identity.task_id", "kandev.ai/task-id", identity.TaskID},
		{"identity.session_id", "kandev.ai/session-id", identity.SessionID},
		{"identity.environment_id", "kandev.ai/environment-id", identity.EnvironmentID},
	}
	labels := map[string]string{
		"app.kubernetes.io/name":       "kandev-agent",
		"app.kubernetes.io/component":  "agent-session",
		"app.kubernetes.io/managed-by": "kandev",
		"app.kubernetes.io/instance":   identity.InstanceID,
	}
	for _, item := range values {
		if item.value == "" || len(validation.IsValidLabelValue(item.value)) > 0 {
			return nil, fieldError(item.path, "must be a non-empty Kubernetes label value")
		}
		labels[item.key] = item.value
	}
	return labels, nil
}

func validateComposition(template *corev1.PodTemplate, profile ProfileConfig, options PodOptions) error {
	if len(validation.IsDNS1123Subdomain(options.Name)) > 0 {
		return fieldError("pod.metadata.name", "must be a valid Pod name")
	}
	if len(validation.IsDNS1123Label(options.Namespace)) > 0 {
		return fieldError("pod.metadata.namespace", "must be a valid namespace")
	}
	if options.AgentctlPort == 0 {
		options.AgentctlPort = DefaultAgentctlPort
	}
	if options.AgentctlPort < 1 || options.AgentctlPort > 65535 {
		return fieldError("pod.agentctl_port", "must be between 1 and 65535")
	}
	if len(options.Command) == 0 {
		return fieldError("pod.command", "is required")
	}
	if !strings.HasPrefix(options.WorkingDir, "/") {
		return fieldError("pod.working_dir", "must be absolute")
	}
	if profile.Platform != PlatformLinuxAMD64 && profile.Platform != PlatformLinuxARM64 {
		return fieldError("config.platform", "must be linux/amd64 or linux/arm64")
	}
	if errs := validation.IsDNS1123Label(profile.MainContainer); len(errs) > 0 {
		return fieldError("config.main_container", "must be a valid container name")
	}
	if err := profile.Workspace.validate(); err != nil {
		return err
	}
	return ValidatePodTemplate(template, profile.MainContainer)
}

func applyPodInvariants(pod *corev1.Pod, platform Platform) {
	pod.Spec.RestartPolicy = corev1.RestartPolicyAlways
	pod.Spec.OS = &corev1.PodOS{Name: corev1.Linux}
	if pod.Spec.AutomountServiceAccountToken == nil {
		value := false
		pod.Spec.AutomountServiceAccountToken = &value
	}
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = make(map[string]string, 2)
	}
	architecture := strings.TrimPrefix(string(platform), "linux/")
	pod.Spec.NodeSelector[corev1.LabelOSStable] = "linux"
	pod.Spec.NodeSelector[corev1.LabelArchStable] = architecture
}

func applyMainContainer(pod *corev1.Pod, mainContainer string, options PodOptions) {
	port := options.AgentctlPort
	if port == 0 {
		port = DefaultAgentctlPort
	}
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != mainContainer {
			continue
		}
		main := &pod.Spec.Containers[i]
		main.Command = append([]string(nil), options.Command...)
		main.Args = append([]string(nil), options.Args...)
		main.WorkingDir = options.WorkingDir
		main.Ports = append(main.Ports, corev1.ContainerPort{
			Name: AgentctlContainerPort, ContainerPort: port, Protocol: corev1.ProtocolTCP,
		})
		main.VolumeMounts = append(main.VolumeMounts, managedVolumeMounts()...)
		return
	}
}

func managedVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{Name: RuntimeVolumeName, MountPath: RuntimeMountPath},
		{Name: AuthVolumeName, MountPath: AuthMountPath},
		{Name: WorkspaceVolumeName, MountPath: WorkspaceMountPath},
	}
}

func managedVolumes(workspace corev1.VolumeSource) []corev1.Volume {
	return []corev1.Volume{
		{Name: RuntimeVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: AuthVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory}}},
		{Name: WorkspaceVolumeName, VolumeSource: workspace},
	}
}

func workspaceVolume(workspace WorkspaceConfig, managedPVCName string) (corev1.VolumeSource, error) {
	switch workspace.Mode {
	case WorkspaceModeEmptyDir:
		return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}, nil
	case WorkspaceModeExistingClaim:
		return claimVolume(workspace.ClaimName), nil
	case WorkspaceModeManagedPVC:
		if len(validation.IsDNS1123Subdomain(managedPVCName)) > 0 {
			return corev1.VolumeSource{}, fieldError("pod.managed_pvc_name", "must be a valid claim name")
		}
		return claimVolume(managedPVCName), nil
	default:
		return corev1.VolumeSource{}, fieldError("config.workspace.mode", "is unsupported")
	}
}

func claimVolume(name string) corev1.VolumeSource {
	return corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name}}
}

func collectWarnings(pod *corev1.Pod) []Warning {
	warnings := make([]Warning, 0)
	if pod.Spec.AutomountServiceAccountToken != nil && *pod.Spec.AutomountServiceAccountToken {
		warnings = append(warnings, warning(
			".template.spec.automountServiceAccountToken",
			"service account token automount is enabled",
		))
	}
	if pod.Spec.HostNetwork {
		warnings = append(warnings, warning(".template.spec.hostNetwork", "host networking is enabled"))
	}
	if pod.Spec.HostPID {
		warnings = append(warnings, warning(".template.spec.hostPID", "host PID namespace is enabled"))
	}
	if pod.Spec.HostIPC {
		warnings = append(warnings, warning(".template.spec.hostIPC", "host IPC namespace is enabled"))
	}
	for i, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil {
			warnings = append(warnings, warning(fmt.Sprintf(".template.spec.volumes[%d].hostPath", i), "host filesystem access is enabled"))
		}
	}
	for i := range pod.Spec.Containers {
		warnings = append(warnings, containerWarnings(&pod.Spec.Containers[i], fmt.Sprintf(".template.spec.containers[%d]", i))...)
	}
	for i := range pod.Spec.InitContainers {
		warnings = append(warnings, containerWarnings(&pod.Spec.InitContainers[i], fmt.Sprintf(".template.spec.initContainers[%d]", i))...)
	}
	return warnings
}

func containerWarnings(container *corev1.Container, path string) []Warning {
	warnings := make([]Warning, 0)
	if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
		warnings = append(warnings, warning(path+".securityContext.privileged", "privileged container is enabled"))
	}
	for i, port := range container.Ports {
		if port.HostPort != 0 {
			warnings = append(warnings, warning(fmt.Sprintf("%s.ports[%d].hostPort", path, i), "host port is enabled"))
		}
	}
	return warnings
}

func warning(suffix, message string) Warning {
	return Warning{Path: podTemplateFieldPrefix + suffix, Message: message}
}
