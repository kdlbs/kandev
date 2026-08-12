//go:build !windows

package lsp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func canonicalExistingFilesystemPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func canonicalPinnedDocumentRootPath(
	access documentRootAccess,
	lexical string,
	resolve func(string) (string, error),
) (string, error) {
	canonical, err := resolve(lexical)
	if err != nil {
		return "", err
	}
	pinnedInfo, err := access.Stat(".")
	if err != nil {
		return "", fmt.Errorf("stat pinned document root: %w", err)
	}
	canonicalInfo, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("stat canonical document root: %w", err)
	}
	if !os.SameFile(pinnedInfo, canonicalInfo) {
		return "", errors.New("document root changed during authorization")
	}
	return canonical, nil
}
