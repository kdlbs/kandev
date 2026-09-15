package kubernetes

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestBuildPersistentVolumeClaimSynthesizesManagedStorage(t *testing.T) {
	t.Parallel()

	workspace := WorkspaceConfig{
		Mode: WorkspaceModeManagedPVC, Size: " 25Gi ", StorageClass: "fast",
		AccessModes: []string{"ReadWriteOnce", "ReadWriteMany"},
	}
	options := PVCOptions{Name: "workspace-pvc", Namespace: "agents", Identity: podOptions().Identity}

	first, err := BuildPersistentVolumeClaim(workspace, options)
	if err != nil {
		t.Fatalf("BuildPersistentVolumeClaim() error = %v", err)
	}
	second, err := BuildPersistentVolumeClaim(workspace, options)
	if err != nil {
		t.Fatalf("second BuildPersistentVolumeClaim() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("BuildPersistentVolumeClaim() is not deterministic")
	}
	if first.APIVersion != "v1" || first.Kind != "PersistentVolumeClaim" || first.Name != options.Name || first.Namespace != options.Namespace {
		t.Fatalf("PVC identity = %#v", first.TypeMeta)
	}
	if len(first.OwnerReferences) != 0 {
		t.Fatalf("PVC ownerReferences = %v, want none", first.OwnerReferences)
	}
	if first.Labels["kandev.ai/session-id"] != options.Identity.SessionID {
		t.Fatalf("PVC labels = %v", first.Labels)
	}
	if first.Spec.StorageClassName == nil || *first.Spec.StorageClassName != "fast" {
		t.Fatalf("storageClassName = %v", first.Spec.StorageClassName)
	}
	wantModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadWriteMany}
	if !reflect.DeepEqual(first.Spec.AccessModes, wantModes) {
		t.Fatalf("accessModes = %v, want %v", first.Spec.AccessModes, wantModes)
	}
	if got := first.Spec.Resources.Requests.Storage(); got == nil || got.Cmp(resource.MustParse("25Gi")) != 0 {
		t.Fatalf("storage request = %v", got)
	}
	if first.Spec.VolumeMode == nil || *first.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		t.Fatalf("volumeMode = %v", first.Spec.VolumeMode)
	}
}

func TestBuildPersistentVolumeClaimRejectsUnmanagedModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []WorkspaceMode{WorkspaceModeEmptyDir, WorkspaceModeExistingClaim} {
		_, err := BuildPersistentVolumeClaim(WorkspaceConfig{Mode: mode, ClaimName: "existing"}, PVCOptions{
			Name: "workspace-pvc", Namespace: "agents", Identity: podOptions().Identity,
		})
		assertFieldPath(t, err, "config.workspace.mode")
	}
}
