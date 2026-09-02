//go:build windows

package lsp

import (
	"errors"
	"fmt"
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

	return canonicalFilesystemPathFromHandle(file)
}

func canonicalFilesystemPathFromHandle(file *os.File) (string, error) {
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

type documentRootHandleAccess interface {
	Open(string) (*os.File, error)
}

func canonicalPinnedDocumentRootPath(
	access documentRootAccess,
	_ string,
	_ func(string) (string, error),
) (string, error) {
	handleAccess, ok := access.(documentRootHandleAccess)
	if !ok {
		return "", errors.New("pinned document root does not expose an identity handle")
	}
	file, err := handleAccess.Open(".")
	if err != nil {
		return "", fmt.Errorf("open pinned document root identity: %w", err)
	}
	defer file.Close()
	pinnedInfo, err := access.Stat(".")
	if err != nil {
		return "", fmt.Errorf("stat pinned document root: %w", err)
	}
	handleInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat pinned document root handle: %w", err)
	}
	if !os.SameFile(pinnedInfo, handleInfo) {
		return "", errors.New("pinned document root identity changed during authorization")
	}
	return canonicalFilesystemPathFromHandle(file)
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
