package kubernetes

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestKubernetesWorkerPresetsCompose(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"minimal", "node-pnpm", "python"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw := readWorkerPreset(t, name)
			template, err := ParsePodTemplate(raw)
			if err != nil {
				t.Fatalf("ParsePodTemplate() error = %v", err)
			}

			profile := ProfileConfig{
				Platform:        PlatformLinuxAMD64,
				MainContainer:   DefaultMainContainerName,
				PodTemplateYAML: raw,
				Workspace: WorkspaceConfig{
					Mode:        WorkspaceModeManagedPVC,
					Size:        "10Gi",
					AccessModes: []string{"ReadWriteOnce"},
				},
			}
			if err := profile.Validate(); err != nil {
				t.Fatalf("ProfileConfig.Validate() error = %v", err)
			}
			pod, _, err := ComposePod(template, profile, PodOptions{
				Name: "kandev-session-1", Namespace: "agents",
				Identity: ResourceIdentity{
					ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "instance-1",
					TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1",
				},
				Command: []string{"/bin/sh"}, Args: []string{"-ceu", "run-managed-entrypoint"},
				WorkingDir: WorkspaceMountPath, ManagedPVCName: "workspace-pvc",
			})
			if err != nil {
				t.Fatalf("ComposePod() error = %v", err)
			}
			assertWorkerPresetTemplate(t, template)
			assertWorkerPresetResources(t, pod)
		})
	}
}

func TestKubernetesWorkerPresetsPreserveRuntimeOwnership(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"minimal", "node-pnpm", "python"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			template, err := ParsePodTemplate(readWorkerPreset(t, name))
			if err != nil {
				t.Fatalf("ParsePodTemplate() error = %v", err)
			}
			main := findWorkerPresetContainer(t, template.Template.Spec.Containers)
			if main.SecurityContext == nil || main.SecurityContext.AllowPrivilegeEscalation == nil ||
				*main.SecurityContext.AllowPrivilegeEscalation {
				t.Fatal("preset must disable privilege escalation")
			}
			if main.SecurityContext.Capabilities == nil || len(main.SecurityContext.Capabilities.Drop) != 1 ||
				main.SecurityContext.Capabilities.Drop[0] != corev1.Capability("ALL") {
				t.Fatalf("preset capabilities = %#v, want drop ALL", main.SecurityContext.Capabilities)
			}
			if main.Env != nil {
				for _, env := range main.Env {
					if env.Name == "HOME" || env.Name == "KANDEV_HOME_DIR" {
						t.Fatalf("preset owns runtime environment %q", env.Name)
					}
				}
			}
		})
	}
}

func readWorkerPreset(t *testing.T, name string) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(source), "../../../../..")
	raw, err := os.ReadFile(filepath.Join(root, "k8s", "presets", name+".yaml"))
	if err != nil {
		t.Fatalf("read %s preset: %v", name, err)
	}
	return string(raw)
}

func assertWorkerPresetTemplate(t *testing.T, template *corev1.PodTemplate) {
	t.Helper()
	if template.Template.Spec.AutomountServiceAccountToken == nil ||
		*template.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("preset must disable service-account token automount")
	}
	if template.Template.Spec.RestartPolicy != "" {
		t.Fatalf("preset owns restartPolicy %q", template.Template.Spec.RestartPolicy)
	}
	if template.Template.Spec.SecurityContext == nil ||
		template.Template.Spec.SecurityContext.RunAsNonRoot == nil ||
		!*template.Template.Spec.SecurityContext.RunAsNonRoot ||
		template.Template.Spec.SecurityContext.RunAsUser == nil ||
		*template.Template.Spec.SecurityContext.RunAsUser != 1000 ||
		template.Template.Spec.SecurityContext.FSGroup == nil ||
		*template.Template.Spec.SecurityContext.FSGroup != 1000 {
		t.Fatalf("preset pod security context = %#v", template.Template.Spec.SecurityContext)
	}
	main := findWorkerPresetContainer(t, template.Template.Spec.Containers)
	if main.Command != nil || main.Args != nil || main.WorkingDir != "" || len(main.Ports) != 0 ||
		len(main.VolumeMounts) != 0 {
		t.Fatalf("preset owns Kandev launch fields: command=%v args=%v workingDir=%q ports=%v mounts=%v", main.Command, main.Args, main.WorkingDir, main.Ports, main.VolumeMounts)
	}
}

func assertWorkerPresetResources(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	main := findWorkerPresetContainer(t, pod.Spec.Containers)
	if main.Resources.Requests.Cpu().Cmp(resource.MustParse("250m")) != 0 ||
		main.Resources.Requests.Memory().Cmp(resource.MustParse("512Mi")) != 0 {
		t.Fatalf("resource requests = %v, want 250m CPU and 512Mi memory", main.Resources.Requests)
	}
	if main.Resources.Limits.Memory().Cmp(resource.MustParse("2Gi")) != 0 {
		t.Fatalf("memory limit = %v, want 2Gi", main.Resources.Limits.Memory())
	}
}

func findWorkerPresetContainer(t *testing.T, containers []corev1.Container) *corev1.Container {
	t.Helper()
	for i := range containers {
		if containers[i].Name == DefaultMainContainerName {
			return &containers[i]
		}
	}
	t.Fatal("preset does not contain kandev-agent")
	return nil
}
