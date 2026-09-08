package kubernetes

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestComposePodPreservesOperatorFieldsAndInjectsOwnedFields(t *testing.T) {
	t.Parallel()

	template, err := ParsePodTemplate(`apiVersion: v1
kind: PodTemplate
template:
  metadata:
    labels:
      team: platform
    annotations:
      example.com/note: retained
  spec:
    serviceAccountName: workload
    automountServiceAccountToken: true
    imagePullSecrets:
      - name: registry
    affinity:
      podAffinity: {}
    tolerations:
      - key: dedicated
        operator: Exists
    containers:
      - name: kandev-agent
        image: example/agent:v1
        resources:
          requests:
            cpu: 100m
      - name: metrics
        image: example/metrics:v2
`)
	if err != nil {
		t.Fatalf("ParsePodTemplate() error = %v", err)
	}
	original := template.DeepCopy()
	profile := validProfile("")
	profile.Workspace = WorkspaceConfig{Mode: WorkspaceModeManagedPVC, Size: "10Gi", AccessModes: []string{"ReadWriteOnce"}}

	pod, warnings, err := ComposePod(template, profile, podOptions())
	if err != nil {
		t.Fatalf("ComposePod() error = %v", err)
	}
	wantWarningPaths := []string{
		"config.pod_template_yaml.template.spec.automountServiceAccountToken",
	}
	if got := warningPaths(warnings); !reflect.DeepEqual(got, wantWarningPaths) {
		t.Fatalf("ComposePod() warning paths = %v, want %v", got, wantWarningPaths)
	}
	if !reflect.DeepEqual(template, original) {
		t.Fatal("ComposePod() mutated input template")
	}
	if pod.Name != "kandev-session-1" || pod.Namespace != "agents" {
		t.Fatalf("pod identity = %s/%s", pod.Namespace, pod.Name)
	}
	if pod.Labels["team"] != "platform" || pod.Annotations["example.com/note"] != "retained" {
		t.Fatalf("operator metadata not preserved: labels=%v annotations=%v", pod.Labels, pod.Annotations)
	}
	if pod.Labels["kandev.ai/session-id"] != "session-1" || pod.Labels["app.kubernetes.io/managed-by"] != "kandev" {
		t.Fatalf("ownership labels = %v", pod.Labels)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Fatalf("restartPolicy = %q", pod.Spec.RestartPolicy)
	}
	if pod.Spec.OS == nil || pod.Spec.OS.Name != corev1.Linux {
		t.Fatalf("spec.os = %#v, want linux", pod.Spec.OS)
	}
	if pod.Spec.NodeSelector[corev1.LabelOSStable] != "linux" || pod.Spec.NodeSelector[corev1.LabelArchStable] != "amd64" {
		t.Fatalf("nodeSelector = %v", pod.Spec.NodeSelector)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || !*pod.Spec.AutomountServiceAccountToken {
		t.Fatal("explicit automountServiceAccountToken=true was not preserved")
	}
	if pod.Spec.ServiceAccountName != "workload" || len(pod.Spec.ImagePullSecrets) != 1 || len(pod.Spec.Tolerations) != 1 {
		t.Fatalf("operator pod fields not preserved: %#v", pod.Spec)
	}
	main := pod.Spec.Containers[0]
	if !reflect.DeepEqual(main.Command, []string{"/bin/sh"}) || !reflect.DeepEqual(main.Args, []string{"-ceu", "run-managed-entrypoint"}) || main.WorkingDir != WorkspaceMountPath {
		t.Fatalf("main launch fields = command=%v args=%v workingDir=%q", main.Command, main.Args, main.WorkingDir)
	}
	if main.Resources.Requests.Cpu().Cmp(resource.MustParse("100m")) != 0 {
		t.Fatalf("main resources not preserved: %v", main.Resources)
	}
	assertManagedPodVolumes(t, pod)
}

func TestComposePodDefaultsServiceAccountTokenOff(t *testing.T) {
	t.Parallel()

	template, err := ParsePodTemplate(validPodTemplate(""))
	if err != nil {
		t.Fatal(err)
	}
	profile := validProfile("")
	pod, _, err := ComposePod(template, profile, podOptions())
	if err != nil {
		t.Fatalf("ComposePod() error = %v", err)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("automountServiceAccountToken default is not false")
	}
}

func TestComposePodWorkspaceVolumeModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace WorkspaceConfig
		wantClaim string
		wantEmpty bool
	}{
		{name: "managed pvc", workspace: WorkspaceConfig{Mode: WorkspaceModeManagedPVC, Size: "10Gi", AccessModes: []string{"ReadWriteOnce"}}, wantClaim: "workspace-pvc"},
		{name: "existing claim", workspace: WorkspaceConfig{Mode: WorkspaceModeExistingClaim, ClaimName: "existing-pvc"}, wantClaim: "existing-pvc"},
		{name: "empty dir", workspace: WorkspaceConfig{Mode: WorkspaceModeEmptyDir}, wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			template, err := ParsePodTemplate(validPodTemplate(""))
			if err != nil {
				t.Fatal(err)
			}
			profile := validProfile("")
			profile.Workspace = tt.workspace
			pod, _, err := ComposePod(template, profile, podOptions())
			if err != nil {
				t.Fatalf("ComposePod() error = %v", err)
			}
			workspace := findVolume(t, pod, WorkspaceVolumeName)
			if tt.wantEmpty && workspace.EmptyDir == nil {
				t.Fatalf("workspace volume = %#v, want emptyDir", workspace)
			}
			if tt.wantClaim != "" && (workspace.PersistentVolumeClaim == nil || workspace.PersistentVolumeClaim.ClaimName != tt.wantClaim) {
				t.Fatalf("workspace volume = %#v, want claim %q", workspace, tt.wantClaim)
			}
		})
	}
}

func TestComposePodWarnsWithoutStrippingPrivilegedHostFields(t *testing.T) {
	t.Parallel()

	template, err := ParsePodTemplate(`apiVersion: v1
kind: PodTemplate
template:
  spec:
    hostNetwork: true
    volumes:
      - name: host
        hostPath:
          path: /var/run
    containers:
      - name: kandev-agent
        image: example/agent:v1
        securityContext:
          privileged: true
`)
	if err != nil {
		t.Fatal(err)
	}
	pod, warnings, err := ComposePod(template, validProfile(""), podOptions())
	if err != nil {
		t.Fatalf("ComposePod() error = %v", err)
	}
	if !pod.Spec.HostNetwork || pod.Spec.Volumes[0].HostPath == nil || pod.Spec.Containers[0].SecurityContext == nil || pod.Spec.Containers[0].SecurityContext.Privileged == nil || !*pod.Spec.Containers[0].SecurityContext.Privileged {
		t.Fatal("privileged operator fields were stripped")
	}
	wantPaths := []string{
		"config.pod_template_yaml.template.spec.hostNetwork",
		"config.pod_template_yaml.template.spec.volumes[0].hostPath",
		"config.pod_template_yaml.template.spec.containers[0].securityContext.privileged",
	}
	if got := warningPaths(warnings); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("warning paths = %v, want %v", got, wantPaths)
	}
}

func podOptions() PodOptions {
	return PodOptions{
		Name: "kandev-session-1", Namespace: "agents",
		Identity: ResourceIdentity{
			ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "instance-1",
			TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1",
		},
		Command: []string{"/bin/sh"}, Args: []string{"-ceu", "run-managed-entrypoint"},
		WorkingDir: WorkspaceMountPath, AgentctlPort: DefaultAgentctlPort, ManagedPVCName: "workspace-pvc",
	}
}

func assertManagedPodVolumes(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	if findVolume(t, pod, RuntimeVolumeName).EmptyDir == nil {
		t.Fatal("runtime volume is not emptyDir")
	}
	auth := findVolume(t, pod, AuthVolumeName)
	if auth.EmptyDir == nil || auth.EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Fatal("auth volume is not memory-backed emptyDir")
	}
	workspace := findVolume(t, pod, WorkspaceVolumeName)
	if workspace.PersistentVolumeClaim == nil || workspace.PersistentVolumeClaim.ClaimName != "workspace-pvc" {
		t.Fatalf("workspace volume = %#v", workspace)
	}
	main := pod.Spec.Containers[0]
	if len(main.VolumeMounts) != 3 || len(main.Ports) != 1 || main.Ports[0].ContainerPort != DefaultAgentctlPort {
		t.Fatalf("main mounts/ports = %#v / %#v", main.VolumeMounts, main.Ports)
	}
}

func findVolume(t *testing.T, pod *corev1.Pod, name string) corev1.Volume {
	t.Helper()
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == name {
			return volume
		}
	}
	t.Fatalf("volume %q not found", name)
	return corev1.Volume{}
}

func warningPaths(warnings []Warning) []string {
	paths := make([]string, len(warnings))
	for i, warning := range warnings {
		paths[i] = warning.Path
	}
	return paths
}
