//go:build windows

package metrics

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func FilesystemIdentity(path string) (string, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("convert path %q: %w", path, err)
	}
	volumePath := make([]uint16, 32768)
	if err := windows.GetVolumePathName(pathPointer, &volumePath[0], uint32(len(volumePath))); err != nil {
		return "", fmt.Errorf("get volume path for %q: %w", path, err)
	}
	volumeMount := windows.UTF16ToString(volumePath)
	volumePointer, err := windows.UTF16PtrFromString(volumeMount)
	if err != nil {
		return "", fmt.Errorf("convert volume path %q: %w", volumeMount, err)
	}
	volumeName := make([]uint16, 1024)
	if err := windows.GetVolumeNameForVolumeMountPoint(volumePointer, &volumeName[0], uint32(len(volumeName))); err != nil {
		return "", fmt.Errorf("get volume identity for %q: %w", path, err)
	}
	return "volume:" + windows.UTF16ToString(volumeName), nil
}
