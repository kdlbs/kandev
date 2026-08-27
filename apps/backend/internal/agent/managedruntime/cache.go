package managedruntime

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const npxExecutionKeyLength = 16

// NpxExecutionCacheKey returns npm's deterministic _npx execution-tree key
// for one trusted package spec. The package spec must be the exact value that
// the managed runtime command passes to npx.
func NpxExecutionCacheKey(packageSpec string) string {
	digest := sha512.Sum512([]byte(packageSpec))
	return hex.EncodeToString(digest[:])[:npxExecutionKeyLength]
}

// ValidateExactPackageSpec accepts only a package name followed by a stable
// version. Cache repair must not accept a path or an unversioned npm selector.
func ValidateExactPackageSpec(packageSpec string) error {
	if strings.TrimSpace(packageSpec) != packageSpec || packageSpec == "" {
		return errors.New("managed runtime package spec is not exact")
	}
	if strings.ContainsAny(packageSpec, "\x00\r\n\\") || strings.Contains(packageSpec, "..") {
		return errors.New("managed runtime package spec is invalid")
	}
	separator := strings.LastIndexByte(packageSpec, '@')
	if separator <= 0 || separator == len(packageSpec)-1 {
		return errors.New("managed runtime package spec must include a version")
	}
	if err := validateManagedPackageName(packageSpec[:separator]); err != nil {
		return err
	}
	if _, err := ParseStableVersion(packageSpec[separator+1:]); err != nil {
		return fmt.Errorf("managed runtime package spec version: %w", err)
	}
	return nil
}

func validateManagedPackageName(packageName string) error {
	if filepath.IsAbs(packageName) || strings.Contains(packageName, ":") || strings.HasSuffix(packageName, "/") {
		return errors.New("managed runtime package spec is path-like")
	}
	if !strings.HasPrefix(packageName, "@") {
		if strings.ContainsAny(packageName, "@/") {
			return errors.New("managed runtime package spec is path-like")
		}
		return nil
	}
	if strings.Contains(packageName[1:], "@") {
		return errors.New("managed runtime package spec has an invalid scope")
	}
	parts := strings.Split(packageName, "/")
	if len(parts) != 2 || parts[0] == "@" || parts[1] == "" {
		return errors.New("managed runtime package spec has an invalid scope")
	}
	return nil
}

// RemoveNpxExecutionTree removes only the _npx tree derived from packageSpec.
// It refuses broad roots and symlinked paths so a cache repair cannot escape
// the exact trusted execution key.
func RemoveNpxExecutionTree(cacheRoot, packageSpec string) error {
	root, err := validateNpmCacheRoot(cacheRoot)
	if err != nil {
		return err
	}
	packageSpec = strings.TrimSpace(packageSpec)
	if packageSpec == "" || strings.ContainsRune(packageSpec, '\x00') || strings.Contains(packageSpec, "..") {
		return errors.New("managed runtime package spec is invalid")
	}

	key := NpxExecutionCacheKey(packageSpec)
	return removeNpxExecutionTree(root, key)
}

func validateNpmCacheRoot(cacheRoot string) (string, error) {
	cacheRoot = strings.TrimSpace(cacheRoot)
	if cacheRoot == "" || !filepath.IsAbs(cacheRoot) {
		return "", fmt.Errorf("npm cache root is not absolute: %q", cacheRoot)
	}
	root := filepath.Clean(cacheRoot)
	if root == string(filepath.Separator) || filepath.Base(root) == "_npx" {
		return "", errors.New("refusing to invalidate npm cache root")
	}
	if info, err := os.Lstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to invalidate execution cache through symlink: %s", root)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("npm cache root is not a directory: %s", root)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect npm cache root: %w", err)
	}
	return root, nil
}
