//go:build windows

package managedruntime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func repairFindExecutable(searchPath, name string) (string, error) {
	extensions := []string{""}
	for _, extension := range strings.Split(os.Getenv("PATHEXT"), ";") {
		extension = strings.TrimSpace(extension)
		if extension != "" {
			extensions = append(extensions, extension)
		}
	}
	for _, directory := range filepath.SplitList(searchPath) {
		if strings.TrimSpace(directory) == "" {
			continue
		}
		for _, extension := range extensions {
			candidate := filepath.Join(directory, name+extension)
			info, err := os.Stat(candidate)
			if err == nil && info.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	return "", errors.New("executable is unavailable")
}
