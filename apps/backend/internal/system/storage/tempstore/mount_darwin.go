//go:build darwin

package tempstore

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type mountReader struct{}

func newMountReader() MountReader { return mountReader{} }

func (mountReader) Identity(path string) (string, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return "", fmt.Errorf("stat temporary mount %s: %w", path, err)
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
