package kubernetes

import (
	"errors"
	"fmt"
	pathpkg "path"
	"reflect"

	corev1 "k8s.io/api/core/v1"
)

// companionWithoutWorkspaceGrant validates and removes the one permitted grant
// from a copy, leaving all other fields subject to the reserved-field checks.
func companionWithoutWorkspaceGrant(container *corev1.Container) (*corev1.Container, *corev1.VolumeMount, error) {
	copy := container.DeepCopy()
	copy.VolumeMounts = nil
	var grant *corev1.VolumeMount
	for _, mount := range container.VolumeMounts {
		if mount.Name == WorkspaceVolumeName {
			allowed := corev1.VolumeMount{Name: WorkspaceVolumeName, MountPath: WorkspaceMountPath, ReadOnly: mount.ReadOnly}
			if grant != nil || !reflect.DeepEqual(mount, allowed) {
				return nil, nil, errors.New("workspace grant must be one direct RO/RW mount at /workspace")
			}
			grant = &allowed
		} else {
			copy.VolumeMounts = append(copy.VolumeMounts, mount)
		}
	}
	if grant != nil {
		for _, mount := range copy.VolumeMounts {
			path := pathpkg.Clean(mount.MountPath)
			if path == "/" || isPathAtOrBelow(WorkspaceMountPath, path) || isPathAtOrBelow(path, WorkspaceMountPath) {
				return nil, nil, errors.New("workspace grant overlaps another mount")
			}
		}
	}
	return copy, grant, nil
}

func validateAdmittedWorkspaceGrants(admitted, desired *corev1.Pod, mainContainer string) error {
	seen := make(map[string]bool)
	for i := range admitted.Spec.Containers {
		container := &admitted.Spec.Containers[i]
		if seen[container.Name] {
			return errors.New("admitted Pod has duplicate container identities")
		}
		seen[container.Name] = true
		if container.Name == mainContainer {
			continue
		}
		_, grant, err := companionWithoutWorkspaceGrant(container)
		if err != nil {
			return err
		}
		requested := admittedContainerByName(desired.Spec.Containers, container.Name)
		var expected *corev1.VolumeMount
		if requested != nil {
			_, expected, err = companionWithoutWorkspaceGrant(requested)
			if err != nil {
				return err
			}
		}
		if !reflect.DeepEqual(grant, expected) {
			return fmt.Errorf("admitted Pod changed workspace grant for %q", container.Name)
		}
	}
	for i := range desired.Spec.Containers {
		container := &desired.Spec.Containers[i]
		if container.Name == mainContainer {
			continue
		}
		_, grant, err := companionWithoutWorkspaceGrant(container)
		if err != nil {
			return err
		}
		if grant != nil && !seen[container.Name] {
			return errors.New("admitted Pod removed a workspace recipient")
		}
	}
	return nil
}
