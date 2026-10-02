//go:build windows

package gocache

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

func cacheMountIdentity(path string) (string, error) {
	input, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("convert Go-cache path %q: %w", path, err)
	}
	buffer := make([]uint16, 32768)
	if err := windows.GetVolumePathName(input, &buffer[0], uint32(len(buffer))); err != nil {
		return "", fmt.Errorf("inspect Go-cache mount for %q: %w", path, err)
	}
	mountpoint := windows.UTF16ToString(buffer)
	if strings.TrimSpace(mountpoint) == "" {
		return "", fmt.Errorf("Go-cache mount identity is unavailable for %q", path)
	}
	return mountpoint, nil
}
