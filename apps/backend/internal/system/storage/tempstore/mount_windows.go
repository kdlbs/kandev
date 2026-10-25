//go:build windows

package tempstore

import (
	"fmt"

	"github.com/kandev/kandev/internal/system/storage"
)

type mountReader struct{}

func newMountReader() MountReader { return mountReader{} }

func (mountReader) Identity(path string) (string, error) {
	volume, err := storage.ResolveVolumeMountPath(path)
	if err != nil {
		return "", fmt.Errorf("resolve temporary volume %s: %w", path, err)
	}
	return volume, nil
}
