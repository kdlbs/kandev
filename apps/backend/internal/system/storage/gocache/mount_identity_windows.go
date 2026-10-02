//go:build windows

package gocache

import (
	"fmt"
	"os"

	"github.com/kandev/kandev/internal/system/storage"
	"golang.org/x/sys/windows"
)

func cacheMountIdentity(path string) (string, error) {
	mountpoint, err := storage.ResolveVolumeMountPath(path)
	if err != nil {
		return "", fmt.Errorf("inspect Go-cache mount for %q: %w", path, err)
	}
	return mountpoint, nil
}

func cacheMountIdentityFromFile(file *os.File) (string, error) {
	buffer := make([]uint16, 32768)
	length, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", fmt.Errorf("inspect final Go-cache path: %w", err)
	}
	if length == 0 || int(length) > len(buffer) {
		return "", fmt.Errorf("final Go-cache path length %d is invalid", length)
	}
	path := windows.UTF16ToString(buffer[:length])
	mountpoint, err := storage.ResolveVolumeMountPath(path)
	if err != nil {
		return "", fmt.Errorf("inspect opened Go-cache mount: %w", err)
	}
	return mountpoint, nil
}
