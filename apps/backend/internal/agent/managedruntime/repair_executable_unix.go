//go:build !windows

package managedruntime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func repairFindExecutable(searchPath, name string) (string, error) {
	for _, directory := range filepath.SplitList(searchPath) {
		if strings.TrimSpace(directory) == "" {
			continue
		}
		candidate := filepath.Join(directory, name)
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", errors.New("executable is unavailable")
}
