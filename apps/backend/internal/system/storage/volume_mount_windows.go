//go:build windows

package storage

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// ResolveVolumeMountPath returns the Windows volume mount path that contains path.
func ResolveVolumeMountPath(path string) (string, error) {
	input, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("convert path %q: %w", path, err)
	}
	buffer := make([]uint16, 32768)
	if err := windows.GetVolumePathName(input, &buffer[0], uint32(len(buffer))); err != nil {
		return "", fmt.Errorf("resolve volume mount path for %q: %w", path, err)
	}
	mountPath := windows.UTF16ToString(buffer)
	if strings.TrimSpace(mountPath) == "" {
		return "", fmt.Errorf("volume mount path is empty for %q", path)
	}
	return filepath.Clean(mountPath), nil
}
