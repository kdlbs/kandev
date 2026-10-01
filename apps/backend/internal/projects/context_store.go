package projects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/worktree"
)

var (
	ErrInvalidContextPath = errors.New("invalid project context path")
	ErrContextConflict    = errors.New("project context changed since it was read")
	ErrContextNotFound    = errors.New("project context file not found")
)

const maxContextFileBytes = 5 << 20

// ContextStore owns the durable filesystem area shared by project tasks.
type ContextStore struct {
	root string
	mu   sync.Mutex
}

type ContextEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
	Size int64  `json:"size,omitempty"`
}

func NewContextStore(root string) *ContextStore {
	return &ContextStore{root: filepath.Clean(root)}
}

func (s *ContextStore) ContextPath(projectID string) (string, error) {
	id, err := canonicalProjectID(projectID)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", fmt.Errorf("resolve project context root: %w", err)
	}
	return filepath.Join(root, id, "context"), nil
}

func (s *ContextStore) Provision(_ context.Context, projectID string) (string, error) {
	root, err := s.ContextPath(projectID)
	if err != nil {
		return "", err
	}
	storageRoot, err := filepath.Abs(s.root)
	if err != nil {
		return "", fmt.Errorf("resolve project context storage root: %w", err)
	}
	contextDir, err := storageworkspaces.CreateDirectoryNoFollow(filepath.Dir(storageRoot), root, 0o700)
	if err != nil {
		return "", fmt.Errorf("open project context: %w", err)
	}
	defer func() { _ = contextDir.Close() }()
	_, err = contextDir.ReadFile("notes.md")
	if err == nil {
		return root, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect initial project notes: %w", err)
	}
	if err := contextDir.CreateFile("notes.md", []byte("# Project notes\n"), 0o600); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create initial project notes: %w", err)
	}
	return root, nil
}

func (s *ContextStore) EnsureTaskLink(taskRoot, projectID, taskID, taskDirName string) error {
	target, err := s.ContextPath(projectID)
	if err != nil {
		return err
	}
	contextDir, err := s.openContextDirectory(target, false)
	if err != nil {
		return fmt.Errorf("project context is unavailable: %w", err)
	}
	if err := contextDir.Close(); err != nil {
		return fmt.Errorf("close project context directory: %w", err)
	}
	_, err = worktree.EnsureOwnedDirectoryLink(taskRoot, "context", target,
		worktree.OwnedDirectoryLinkOwner{TaskID: taskID, TaskDirName: taskDirName})
	return err
}

func RemoveTaskContextLink(taskRoot string) error {
	return worktree.RemoveOwnedDirectoryLink(taskRoot, "context")
}

func (s *ContextStore) RemoveProject(projectID string) error {
	path, err := s.ContextPath(projectID)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return fmt.Errorf("resolve project context storage root: %w", err)
	}
	target, err := filepath.Rel(root, path)
	if err != nil || target == "." || filepath.IsAbs(target) || strings.HasPrefix(target, ".."+string(filepath.Separator)) {
		return ErrInvalidContextPath
	}
	if err := storageworkspaces.RemoveDirectoryNoFollow(context.Background(), root, path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove project context: %w", err)
	}
	return nil
}

func (s *ContextStore) List(projectID, relative string) ([]ContextEntry, error) {
	directory, clean, err := s.resolvePath(projectID, relative, true)
	if err != nil {
		return nil, err
	}
	contextDir, err := s.openContextDirectory(directory, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrContextNotFound
		}
		return nil, fmt.Errorf("open project context directory: %w", err)
	}
	defer func() { _ = contextDir.Close() }()
	entries, err := contextDir.ReadContextEntries()
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrContextNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("list project context: %w", err)
	}
	result := make([]ContextEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Mode&os.ModeSymlink != 0 || (!entry.Mode.IsDir() && !entry.Mode.IsRegular()) {
			continue
		}
		entryPath := entry.Name
		if clean != "" {
			entryPath = filepath.ToSlash(filepath.Join(clean, entryPath))
		}
		kind := "file"
		if entry.Mode.IsDir() {
			kind = "directory"
		}
		result = append(result, ContextEntry{Name: entry.Name, Path: entryPath, Kind: kind, Size: entry.Size})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *ContextStore) ReadFile(projectID, relative string) (string, string, error) {
	path, _, err := s.resolvePath(projectID, relative, false)
	if err != nil {
		return "", "", err
	}
	data, err := s.readResolvedFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", ErrContextNotFound
	}
	if err != nil {
		return "", "", err
	}
	return string(data), contentHash(data), nil
}

func (s *ContextStore) WriteFile(projectID, relative, expectedHash string, content []byte) (string, error) {
	if len(content) > maxContextFileBytes {
		return "", fmt.Errorf("%w: file exceeds %d bytes", ErrInvalidContextPath, maxContextFileBytes)
	}
	path, _, err := s.resolvePath(projectID, relative, false)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeResolvedFile(path, expectedHash, content)
}

func (s *ContextStore) writeResolvedFile(path, expectedHash string, content []byte) (string, error) {
	contextDir, err := s.openContextParent(path, true)
	if err != nil {
		return "", fmt.Errorf("open project context directory: %w", err)
	}
	defer func() { _ = contextDir.Close() }()
	name := filepath.Base(path)
	if err := checkContextWriteVersion(contextDir, name, expectedHash); err != nil {
		return "", err
	}
	if err := contextDir.WriteFileAtomic(name, content, 0o600); err != nil {
		return "", fmt.Errorf("commit project context write: %w", err)
	}
	return contentHash(content), nil
}

func checkContextWriteVersion(directory storageworkspaces.DirectoryHandle, name, expectedHash string) error {
	current, readErr := readContextFile(directory, name)
	switch {
	case errors.Is(readErr, os.ErrNotExist):
		if expectedHash != "" {
			return ErrContextConflict
		}
	case readErr != nil:
		return readErr
	case expectedHash == "" || contentHash(current) != expectedHash:
		return ErrContextConflict
	}
	return nil
}

func (s *ContextStore) resolvePath(projectID, relative string, directory bool) (string, string, error) {
	root, err := s.ContextPath(projectID)
	if err != nil {
		return "", "", err
	}
	clean, err := cleanContextRelativePath(relative, directory)
	if err != nil {
		return "", "", err
	}
	path := root
	if clean != "" {
		path = filepath.Join(root, filepath.FromSlash(clean))
	}
	if err := rejectContainedPathSymlinks(root, path, directory); err != nil {
		return "", "", err
	}
	return path, clean, nil
}

func (s *ContextStore) readResolvedFile(path string) ([]byte, error) {
	directory, err := s.openContextParent(path, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()
	return readContextFile(directory, filepath.Base(path))
}

func readContextFile(directory storageworkspaces.DirectoryHandle, name string) ([]byte, error) {
	data, err := directory.ReadFileLimit(name, maxContextFileBytes)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return data, err
	}
	if errors.Is(err, ErrInvalidContextPath) {
		return nil, err
	}
	return nil, fmt.Errorf("%w: %v", ErrInvalidContextPath, err)
}

func (s *ContextStore) openContextDirectory(path string, create bool) (storageworkspaces.DirectoryHandle, error) {
	root, err := filepath.Abs(s.root)
	if err != nil {
		return nil, fmt.Errorf("resolve project context storage root: %w", err)
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve project context directory: %w", err)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, ErrInvalidContextPath
	}
	var handle storageworkspaces.DirectoryHandle
	if create {
		handle, err = storageworkspaces.CreateDirectoryNoFollow(root, target, 0o700)
	} else {
		handle, err = storageworkspaces.OpenDirectoryNoFollow(root, target)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidContextPath, err)
	}
	return handle, nil
}

func (s *ContextStore) openContextParent(path string, create bool) (storageworkspaces.DirectoryHandle, error) {
	return s.openContextDirectory(filepath.Dir(path), create)
}

func cleanContextRelativePath(relative string, allowRoot bool) (string, error) {
	if relative == "" || relative == "." {
		if allowRoot {
			return "", nil
		}
		return "", ErrInvalidContextPath
	}
	if strings.ContainsRune(relative, '\x00') || filepath.IsAbs(relative) || strings.Contains(relative, `\`) {
		return "", ErrInvalidContextPath
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return "", ErrInvalidContextPath
	}
	return clean, nil
}

func rejectContainedPathSymlinks(root, path string, allowMissingLeaf bool) error {
	rel, err := containedRelativePath(root, path)
	if err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return err
	}
	current := root
	if rel == "." {
		return nil
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		if err := validateContainedPathComponent(current, index, len(parts), allowMissingLeaf); err != nil {
			return err
		}
	}
	return nil
}

func containedRelativePath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrInvalidContextPath
	}
	return rel, nil
}

func validateContainedPathComponent(path string, index, length int, allowMissingLeaf bool) error {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if allowMissingLeaf || index < length-1 {
			return nil
		}
		return nil
	case err != nil:
		return fmt.Errorf("inspect project context path: %w", err)
	case info.Mode()&os.ModeSymlink != 0 || worktree.IsDirectoryLink(path) || index < length-1 && !info.IsDir():
		return ErrInvalidContextPath
	default:
		return nil
	}
}

func rejectSymlinkAncestors(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	current := volume + string(filepath.Separator)
	rel := strings.TrimPrefix(absolute, current)
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || worktree.IsDirectoryLink(current) {
			return ErrInvalidContextPath
		}
	}
	return nil
}

func contentHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func canonicalProjectID(projectID string) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(projectID))
	if err != nil || id.String() != strings.ToLower(strings.TrimSpace(projectID)) {
		return "", ErrInvalidContextPath
	}
	return id.String(), nil
}
