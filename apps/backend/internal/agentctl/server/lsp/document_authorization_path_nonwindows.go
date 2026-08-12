//go:build !windows

package lsp

import "path/filepath"

func canonicalExistingFilesystemPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
