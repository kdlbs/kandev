//go:build windows

package tempstore

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type mountReader struct{}

func newMountReader() MountReader { return mountReader{} }

func (mountReader) Identity(path string) (string, error) {
	input, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 32768)
	if err := windows.GetVolumePathName(input, &buffer[0], uint32(len(buffer))); err != nil {
		return "", fmt.Errorf("resolve temporary volume %s: %w", path, err)
	}
	volume := windows.UTF16ToString(buffer)
	if strings.TrimSpace(volume) == "" {
		return "", fmt.Errorf("temporary volume is empty for %s", path)
	}
	return filepath.Clean(volume), nil
}
