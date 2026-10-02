//go:build darwin

package gocache

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func cacheMountIdentity(path string) (string, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return "", fmt.Errorf("inspect Go-cache mount for %q: %w", path, err)
	}
	mountpoint := strings.TrimRight(stringFromBytes(stat.Mntonname[:]), string(filepath.Separator))
	if mountpoint == "" {
		mountpoint = string(filepath.Separator)
	}
	return filepath.Clean(mountpoint), nil
}

func stringFromBytes(value []byte) string {
	for index, item := range value {
		if item == 0 {
			return string(value[:index])
		}
	}
	return string(value)
}
