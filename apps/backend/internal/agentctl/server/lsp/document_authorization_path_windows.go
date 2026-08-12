//go:build windows

package lsp

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func canonicalExistingFilesystemPath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]uint16, 256)
	for {
		length, pathErr := windows.GetFinalPathNameByHandle(
			windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 0,
		)
		if pathErr != nil {
			return "", pathErr
		}
		if length < uint32(len(buffer)) {
			return filepath.Clean(normalizeWindowsFinalPath(windows.UTF16ToString(buffer[:length]))), nil
		}
		buffer = make([]uint16, length+1)
	}
}

func normalizeWindowsFinalPath(path string) string {
	const (
		uncPrefix  = `\\?\UNC\`
		longPrefix = `\\?\`
	)
	if strings.HasPrefix(path, uncPrefix) {
		return `\\` + strings.TrimPrefix(path, uncPrefix)
	}
	return strings.TrimPrefix(path, longPrefix)
}
