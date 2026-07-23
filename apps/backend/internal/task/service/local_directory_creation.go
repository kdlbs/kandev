package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var (
	ErrInvalidDirectoryCreation = errors.New("invalid directory creation")
	ErrDirectoryAlreadyExists   = errors.New("directory already exists")
)

// CreateDirectory creates one child directory for the local folder navigator.
func (s *Service) CreateDirectory(
	ctx context.Context,
	parentPath string,
	name string,
) (DirectoryListing, error) {
	name = strings.TrimSpace(name)
	if err := validateLocalRepositoryName(name); err != nil {
		return DirectoryListing{}, fmt.Errorf("%w: name must be a single path segment", ErrInvalidDirectoryCreation)
	}
	parentPath, err := canonicalLocalRepositoryParent(parentPath)
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("%w: parent directory cannot be accessed", ErrInvalidDirectoryCreation)
	}
	targetPath := filepath.Join(parentPath, name)
	if _, err := lstatLocalRepositoryPath(targetPath); err == nil {
		return DirectoryListing{}, ErrDirectoryAlreadyExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return DirectoryListing{}, fmt.Errorf("%w: target path cannot be inspected", ErrInvalidDirectoryCreation)
	}
	createdPath, err := createMissingLocalDirectories(parentPath, []string{name})
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("create directory: %w", err)
	}
	return s.ListDirectory(ctx, createdPath)
}

func ensureLocalRepositoryDirectory(path string) (string, error) {
	existingPath, missingSegments, err := nearestExistingLocalDirectory(path)
	if err != nil {
		return "", err
	}
	canonicalPath, err := filepath.EvalSymlinks(existingPath)
	if err != nil {
		return "", err
	}
	if err := validateLocalRepositoryParentDirectory(canonicalPath); err != nil {
		return "", err
	}
	if len(missingSegments) == 0 {
		return canonicalPath, nil
	}
	createdPath, err := createMissingLocalDirectories(canonicalPath, missingSegments)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(createdPath)
}

func nearestExistingLocalDirectory(path string) (string, []string, error) {
	currentPath := filepath.Clean(path)
	missingSegments := make([]string, 0)
	for {
		resolvedPath, err := filepath.EvalSymlinks(currentPath)
		if err == nil {
			info, statErr := statLocalRepositoryPath(resolvedPath)
			if statErr != nil {
				return "", nil, statErr
			}
			if !info.IsDir() {
				return "", nil, fmt.Errorf("not a directory: %s", currentPath)
			}
			slices.Reverse(missingSegments)
			return resolvedPath, missingSegments, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", nil, err
		}
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return "", nil, err
		}
		missingSegments = append(missingSegments, filepath.Base(currentPath))
		currentPath = parentPath
	}
}

func createMissingLocalDirectories(parentPath string, segments []string) (string, error) {
	currentRoot, err := os.OpenRoot(parentPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = currentRoot.Close() }()

	currentPath := parentPath
	for _, segment := range segments {
		if err := currentRoot.Mkdir(segment, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		info, err := currentRoot.Lstat(segment)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			!localRepositoryDirectoryOwnedByProcess(info) || localRepositoryParentSharedWritable(info) {
			return "", fmt.Errorf("created directory is not trusted")
		}
		nextRoot, err := currentRoot.OpenRoot(segment)
		if err != nil {
			return "", err
		}
		if err := currentRoot.Close(); err != nil {
			_ = nextRoot.Close()
			return "", err
		}
		currentRoot = nextRoot
		currentPath = filepath.Join(currentPath, segment)
	}
	return currentPath, nil
}
