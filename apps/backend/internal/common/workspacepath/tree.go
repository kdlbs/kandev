// Package workspacepath validates path identities used by workspace services.
package workspacepath

import (
	"errors"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// ErrTreePathNotRelative identifies a path that cannot address the workspace tree.
var ErrTreePathNotRelative = errors.New("file tree path must be workspace-relative")

// ValidateTreePath accepts the empty workspace root and canonical slash-separated relative paths.
func ValidateTreePath(requested string) error {
	if requested == "" {
		return nil
	}
	if strings.IndexByte(requested, 0) >= 0 ||
		strings.Contains(requested, `\`) ||
		filepath.IsAbs(requested) ||
		pathpkg.IsAbs(filepath.ToSlash(requested)) ||
		filepath.VolumeName(requested) != "" ||
		hasWindowsDrivePrefix(requested) ||
		hasURIScheme(requested) {
		return ErrTreePathNotRelative
	}
	cleaned := pathpkg.Clean(requested)
	if cleaned != requested || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ErrTreePathNotRelative
	}
	return nil
}

func hasURIScheme(path string) bool {
	colon := strings.IndexByte(path, ':')
	if colon <= 0 || !isASCIILetter(path[0]) {
		return false
	}
	for index := 1; index < colon; index++ {
		char := path[index]
		if !isASCIILetter(char) && (char < '0' || char > '9') && char != '+' && char != '-' && char != '.' {
			return false
		}
	}
	return true
}

func hasWindowsDrivePrefix(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	first := path[0]
	return isASCIILetter(first)
}

func isASCIILetter(char byte) bool {
	return char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z'
}
