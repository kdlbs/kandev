package workspaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// DirectoryHandle keeps a directory pinned while a worktree decision is made.
// Implementations open every path component without following links and use
// the resulting descriptor or native handle for subsequent reads, writes, and
// removal. This prevents a path replacement between validation and the side
// effect from redirecting the operation to another task root.
type DirectoryHandle interface {
	Close() error
	VerifyPath(path string) error
	IsValidWorktree() bool
	RemoveDirectory(ctx context.Context) error
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, mode os.FileMode) error
}

// WriteOwnershipMarkerNoFollow writes an ownership marker through an already
// opened task-root handle. The caller must verify the handle still identifies
// the expected lexical path before calling this function.
func WriteOwnershipMarkerNoFollow(handle DirectoryHandle, marker OwnershipMarker) error {
	if handle == nil {
		return errors.New("workspace ownership marker requires an opened directory")
	}
	if err := normalizeOwnershipMarker(&marker); err != nil {
		return err
	}
	matched, err := existingMarkerMatchesHandle(handle, marker)
	if err != nil {
		return err
	}
	if matched {
		return nil
	}
	encoded, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("encode workspace ownership marker: %w", err)
	}
	if err := handle.WriteFile(OwnershipMarkerFilename, encoded, 0o600); err != nil {
		return fmt.Errorf("install workspace ownership marker: %w", err)
	}
	return nil
}

func existingMarkerMatchesHandle(handle DirectoryHandle, marker OwnershipMarker) (bool, error) {
	encoded, err := handle.ReadFile(OwnershipMarkerFilename)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var existing OwnershipMarker
	if err := json.Unmarshal(encoded, &existing); err != nil {
		return false, fmt.Errorf("decode workspace ownership marker: %w", err)
	}
	if existing.TaskID == "" || existing.TaskDirName == "" ||
		(existing.LayoutVersion != LayoutVersionSemantic && existing.LayoutVersion != LayoutVersionScratch) {
		return false, errors.New("invalid workspace ownership marker fields")
	}
	if existing.TaskID != marker.TaskID || existing.TaskDirName != marker.TaskDirName || existing.LayoutVersion != marker.LayoutVersion {
		return false, errors.New("workspace ownership marker conflicts with requested task root")
	}
	if existing.WorkspaceID != "" && marker.WorkspaceID != "" && existing.WorkspaceID != marker.WorkspaceID {
		return false, errors.New("workspace ownership marker conflicts with requested workspace")
	}
	return true, nil
}

func validateDirectoryEntryName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("invalid directory entry name: %q", name)
	}
	return nil
}
