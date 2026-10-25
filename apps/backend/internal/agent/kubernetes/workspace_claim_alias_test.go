package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestWorkspaceGrantAdmissionRejectsClaimAlias(t *testing.T) {
	for _, mode := range []WorkspaceMode{WorkspaceModeManagedPVC, WorkspaceModeExistingClaim} {
		t.Run(string(mode), func(t *testing.T) {
			profile := validProfile("")
			profile.Workspace.Mode = mode
			profile.Workspace.Size = "1Gi"
			profile.Workspace.AccessModes = []string{"ReadWriteOnce"}
			if mode == WorkspaceModeExistingClaim {
				profile.Workspace.ClaimName = "existing-workspace"
				profile.Workspace.Size = ""
				profile.Workspace.AccessModes = nil
			}
			desired, _, err := ComposePod(workspaceGrantTemplate(t, true), profile, podOptions())
			if err != nil {
				t.Fatal(err)
			}
			var workspaceClaim string
			for _, volume := range desired.Spec.Volumes {
				if volume.Name == WorkspaceVolumeName {
					workspaceClaim = volume.PersistentVolumeClaim.ClaimName
				}
			}
			for _, claim := range []string{workspaceClaim, "unrelated-cache"} {
				admitted := desired.DeepCopy()
				admitted.Spec.Volumes = append(admitted.Spec.Volumes, corev1.Volume{
					Name: "companion-cache",
					VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claim,
					}},
				})
				admitted.Spec.Containers[1].VolumeMounts = append(admitted.Spec.Containers[1].VolumeMounts,
					corev1.VolumeMount{Name: "companion-cache", MountPath: "/cache", ReadOnly: false})
				err := ValidateAdmittedPod(admitted, desired, DefaultMainContainerName)
				if claim == workspaceClaim && err == nil {
					t.Error("accepted a writable alias of the read-only workspace grant")
				}
				if claim != workspaceClaim && err != nil {
					t.Errorf("unrelated claim rejected: %v", err)
				}
				template := workspaceGrantTemplate(t, true)
				template.Template.Spec.Volumes = []corev1.Volume{admitted.Spec.Volumes[len(admitted.Spec.Volumes)-1]}
				template.Template.Spec.Containers[1] = admitted.Spec.Containers[1]
				_, _, err = ComposePod(template, profile, podOptions())
				if (err != nil) != (claim == workspaceClaim) {
					t.Errorf("composition for claim %q: %v", claim, err)
				}
			}
		})
	}
}
