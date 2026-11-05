package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ValidateFilesystemOperation authorizes one file operation without changing policy.
func ValidateFilesystemOperation(target string, writable bool, ceiling Grants, protectedRoots []string) error {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target || len(target) > 4096 || strings.ContainsFunc(target, unicode.IsControl) {
		return ErrInvalidPolicy
	}
	resolved, err := filepath.EvalSymlinks(target)
	if errors.Is(err, os.ErrNotExist) && writable {
		parent, parentErr := canonicalPath(filepath.Dir(target))
		if parentErr != nil {
			return parentErr
		}
		resolved, err = filepath.Join(parent, filepath.Base(target)), nil
	}
	if err != nil || resolved != target {
		return ErrInvalidPolicy
	}
	roots, err := canonicalProtectedRoots(protectedRoots)
	if err != nil {
		return err
	}
	for _, root := range roots {
		if within(target, root) || within(root, target) {
			return ErrInvalidPolicy
		}
	}
	return WithinCeiling(Grants{Filesystem: []FilesystemGrant{{Path: target, Writable: writable}}}, ceiling)
}
