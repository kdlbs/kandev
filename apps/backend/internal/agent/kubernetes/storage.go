package kubernetes

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

type PVCOptions struct {
	Name      string
	Namespace string
	Identity  ResourceIdentity
}

func BuildPersistentVolumeClaim(workspace WorkspaceConfig, options PVCOptions) (*corev1.PersistentVolumeClaim, error) {
	if workspace.Mode != WorkspaceModeManagedPVC {
		return nil, fieldError("config.workspace.mode", "must be managed_pvc to create a claim")
	}
	if err := workspace.validateManagedPVC(); err != nil {
		return nil, err
	}
	if len(validation.IsDNS1123Subdomain(options.Name)) > 0 {
		return nil, fieldError("pvc.metadata.name", "must be a valid claim name")
	}
	if len(validation.IsDNS1123Label(options.Namespace)) > 0 {
		return nil, fieldError("pvc.metadata.namespace", "must be a valid namespace")
	}
	labels, err := OwnershipLabels(options.Identity)
	if err != nil {
		return nil, err
	}
	quantity, err := resource.ParseQuantity(strings.TrimSpace(workspace.Size))
	if err != nil {
		return nil, fieldError("config.workspace.size", "must be a positive Kubernetes quantity")
	}
	volumeMode := corev1.PersistentVolumeFilesystem
	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
		ObjectMeta: metav1.ObjectMeta{
			Name: options.Name, Namespace: options.Namespace, Labels: labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes(workspace.AccessModes),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: quantity},
			},
			VolumeMode: &volumeMode,
		},
	}
	if workspace.StorageClass != "" {
		storageClass := workspace.StorageClass
		pvc.Spec.StorageClassName = &storageClass
	}
	return pvc, nil
}

func accessModes(values []string) []corev1.PersistentVolumeAccessMode {
	modes := make([]corev1.PersistentVolumeAccessMode, len(values))
	for i, value := range values {
		modes[i] = corev1.PersistentVolumeAccessMode(value)
	}
	return modes
}
