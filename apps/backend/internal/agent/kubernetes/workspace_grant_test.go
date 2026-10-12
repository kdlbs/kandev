package kubernetes

import (
	corev1 "k8s.io/api/core/v1"
	"reflect"
	"testing"
)

func workspaceGrantTemplate(t *testing.T, readOnly bool) *corev1.PodTemplate {
	t.Helper()
	template, err := ParsePodTemplate(validPodTemplate(""))
	if err != nil {
		t.Fatal(err)
	}
	template.Template.Spec.Containers = append(template.Template.Spec.Containers, corev1.Container{Name: "docker-engine", Image: "example/daemon:v1", VolumeMounts: []corev1.VolumeMount{{Name: WorkspaceVolumeName, MountPath: WorkspaceMountPath, ReadOnly: readOnly}}})
	return template
}

func TestWorkspaceGrantTemplateValidation(t *testing.T) {
	for _, ro := range []bool{false, true} {
		if err := ValidatePodTemplate(workspaceGrantTemplate(t, ro), DefaultMainContainerName); err != nil {
			t.Errorf("readOnly=%v: %v", ro, err)
		}
	}
	mutations := map[string]func(*corev1.PodSpec){
		"redirect":        func(s *corev1.PodSpec) { s.Containers[1].VolumeMounts[0].MountPath = "/other" },
		"normalized path": func(s *corev1.PodSpec) { s.Containers[1].VolumeMounts[0].MountPath = "/workspace/" },
		"subpath":         func(s *corev1.PodSpec) { s.Containers[1].VolumeMounts[0].SubPath = "repo" },
		"expression":      func(s *corev1.PodSpec) { s.Containers[1].VolumeMounts[0].SubPathExpr = "$(REPO)" },
		"propagation": func(s *corev1.PodSpec) {
			v := corev1.MountPropagationNone
			s.Containers[1].VolumeMounts[0].MountPropagation = &v
		},
		"recursive": func(s *corev1.PodSpec) {
			v := corev1.RecursiveReadOnlyDisabled
			s.Containers[1].VolumeMounts[0].RecursiveReadOnly = &v
		},
		"duplicate": func(s *corev1.PodSpec) {
			s.Containers[1].VolumeMounts = append(s.Containers[1].VolumeMounts, s.Containers[1].VolumeMounts[0])
		},
		"overlap": func(s *corev1.PodSpec) {
			s.Containers[1].VolumeMounts = append(s.Containers[1].VolumeMounts, corev1.VolumeMount{Name: "extra", MountPath: "/workspace/cache"})
		},
		"ancestor": func(s *corev1.PodSpec) {
			s.Containers[1].VolumeMounts = append(s.Containers[1].VolumeMounts, corev1.VolumeMount{Name: "extra", MountPath: "/"})
		},
		"auth":              func(s *corev1.PodSpec) { s.Containers[1].VolumeMounts[0].Name = AuthVolumeName },
		"runtime":           func(s *corev1.PodSpec) { s.Containers[1].VolumeMounts[0].Name = RuntimeVolumeName },
		"volume definition": func(s *corev1.PodSpec) { s.Volumes = append(s.Volumes, corev1.Volume{Name: WorkspaceVolumeName}) },
		"device": func(s *corev1.PodSpec) {
			s.Containers[1].VolumeDevices = []corev1.VolumeDevice{{Name: WorkspaceVolumeName, DevicePath: "/dev/workspace"}}
		},
		"init": func(s *corev1.PodSpec) {
			s.InitContainers = []corev1.Container{s.Containers[1]}
			s.Containers = s.Containers[:1]
		},
		"ephemeral": func(s *corev1.PodSpec) {
			s.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon(s.Containers[1])}}
			s.Containers = s.Containers[:1]
		},
		"main": func(s *corev1.PodSpec) {
			s.Containers[0].VolumeMounts = s.Containers[1].VolumeMounts
			s.Containers = s.Containers[:1]
		},
		"duplicate recipient": func(s *corev1.PodSpec) { s.Containers = append(s.Containers, s.Containers[1]) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			template := workspaceGrantTemplate(t, false)
			mutate(&template.Template.Spec)
			if err := ValidatePodTemplate(template, DefaultMainContainerName); err == nil {
				t.Fatal("accepted invalid grant")
			}
		})
	}
}

func TestWorkspaceGrantComposition(t *testing.T) {
	for _, mode := range []WorkspaceMode{WorkspaceModeEmptyDir, WorkspaceModeManagedPVC, WorkspaceModeExistingClaim} {
		for _, ro := range []bool{false, true} {
			template := workspaceGrantTemplate(t, ro)
			original := template.DeepCopy()
			profile := validProfile("")
			profile.Workspace.Mode = mode
			if mode == WorkspaceModeManagedPVC {
				profile.Workspace.Size = "1Gi"
				profile.Workspace.AccessModes = []string{"ReadWriteOnce"}
			}
			if mode == WorkspaceModeExistingClaim {
				profile.Workspace.ClaimName = "existing-workspace"
				profile.Workspace.Size = ""
				profile.Workspace.AccessModes = nil
			}
			pod, _, err := ComposePod(template, profile, podOptions())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(pod.Spec.Containers[1], original.Template.Spec.Containers[1]) || !reflect.DeepEqual(template, original) {
				t.Fatal("grant or input changed")
			}
			if err := ValidateAdmittedPod(pod.DeepCopy(), pod, DefaultMainContainerName); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestWorkspaceGrantAdmission(t *testing.T) {
	mutations := map[string]func(*corev1.Pod){
		"removed mount":     func(p *corev1.Pod) { p.Spec.Containers[1].VolumeMounts = nil },
		"removed recipient": func(p *corev1.Pod) { p.Spec.Containers = p.Spec.Containers[:1] },
		"renamed":           func(p *corev1.Pod) { p.Spec.Containers[1].Name = "other" },
		"mode":              func(p *corev1.Pod) { p.Spec.Containers[1].VolumeMounts[0].ReadOnly = true },
		"duplicate mount": func(p *corev1.Pod) {
			p.Spec.Containers[1].VolumeMounts = append(p.Spec.Containers[1].VolumeMounts, p.Spec.Containers[1].VolumeMounts[0])
		},
		"duplicate recipient": func(p *corev1.Pod) { p.Spec.Containers = append(p.Spec.Containers, p.Spec.Containers[1]) },
		"added": func(p *corev1.Pod) {
			c := *p.Spec.Containers[1].DeepCopy()
			c.Name = "other"
			p.Spec.Containers = append(p.Spec.Containers, c)
		},
		"init transfer": func(p *corev1.Pod) {
			p.Spec.InitContainers = []corev1.Container{p.Spec.Containers[1]}
			p.Spec.Containers = p.Spec.Containers[:1]
		},
		"ephemeral transfer": func(p *corev1.Pod) {
			p.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon(p.Spec.Containers[1])}}
			p.Spec.Containers = p.Spec.Containers[:1]
		},
	}
	desired, _ := admittedPodFixture(t)
	desired.Spec.Containers = append(desired.Spec.Containers, workspaceGrantTemplate(t, false).Template.Spec.Containers[1])
	if err := ValidateAdmittedPod(desired.DeepCopy(), desired, DefaultMainContainerName); err != nil {
		t.Errorf("unchanged grant: %v", err)
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			admitted := desired.DeepCopy()
			mutate(admitted)
			if err := ValidateAdmittedPod(admitted, desired, DefaultMainContainerName); err == nil {
				t.Fatal("accepted mutated grant")
			}
		})
	}
	admitted := desired.DeepCopy()
	admitted.Spec.Containers = append(admitted.Spec.Containers, corev1.Container{Name: "metrics", Image: "example/metrics:v1"})
	admitted.Spec.Containers[0], admitted.Spec.Containers[1] = admitted.Spec.Containers[1], admitted.Spec.Containers[0]
	if err := ValidateAdmittedPod(admitted, desired, DefaultMainContainerName); err != nil {
		t.Errorf("compatible addition/reordering: %v", err)
	}
}
