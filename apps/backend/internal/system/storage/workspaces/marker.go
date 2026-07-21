package workspaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	OwnershipMarkerFilename = ".kandev-workspace.json"
	quarantineManifestName  = ".kandev-quarantine.json"
)

func WriteOwnershipMarker(root string, marker OwnershipMarker) error {
	if err := normalizeOwnershipMarker(&marker); err != nil {
		return err
	}
	if err := rejectRootSymlink(root); err != nil {
		return err
	}
	matched, err := existingMarkerMatches(root, marker)
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
	path := filepath.Join(root, OwnershipMarkerFilename)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace ownership marker is a symlink: %s", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect workspace ownership marker: %w", err)
	}
	tmp, err := os.CreateTemp(root, ".kandev-workspace-*")
	if err != nil {
		return fmt.Errorf("create workspace ownership marker: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("install workspace ownership marker: %w", err)
	}
	return nil
}

func normalizeOwnershipMarker(marker *OwnershipMarker) error {
	if marker.TaskID == "" || marker.TaskDirName == "" {
		return errors.New("workspace ownership marker requires task_id and task_dir_name")
	}
	if marker.LayoutVersion != LayoutVersionSemantic && marker.LayoutVersion != LayoutVersionScratch {
		return fmt.Errorf("unsupported workspace layout version %d", marker.LayoutVersion)
	}
	if marker.CreatedAt.IsZero() {
		marker.CreatedAt = time.Now().UTC()
	} else {
		marker.CreatedAt = marker.CreatedAt.UTC()
	}
	return nil
}

func existingMarkerMatches(root string, marker OwnershipMarker) (bool, error) {
	existing, found, err := readOwnershipMarker(root)
	if err != nil || !found {
		return found, err
	}
	if existing.TaskID != marker.TaskID || existing.TaskDirName != marker.TaskDirName || existing.LayoutVersion != marker.LayoutVersion {
		return false, errors.New("workspace ownership marker conflicts with requested task root")
	}
	if existing.WorkspaceID != "" && marker.WorkspaceID != "" && existing.WorkspaceID != marker.WorkspaceID {
		return false, errors.New("workspace ownership marker conflicts with requested workspace")
	}
	return true, nil
}

func readOwnershipMarker(root string) (OwnershipMarker, bool, error) {
	path := filepath.Join(root, OwnershipMarkerFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return OwnershipMarker{}, false, nil
	}
	if err != nil {
		return OwnershipMarker{}, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return OwnershipMarker{}, false, fmt.Errorf("invalid workspace ownership marker: %s", path)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return OwnershipMarker{}, false, err
	}
	var marker OwnershipMarker
	if err := json.Unmarshal(encoded, &marker); err != nil {
		return OwnershipMarker{}, false, fmt.Errorf("decode workspace ownership marker: %w", err)
	}
	if marker.TaskID == "" || marker.TaskDirName == "" ||
		(marker.LayoutVersion != LayoutVersionSemantic && marker.LayoutVersion != LayoutVersionScratch) {
		return OwnershipMarker{}, false, errors.New("invalid workspace ownership marker fields")
	}
	return marker, true, nil
}

func ReadOwnershipMarker(root string) (OwnershipMarker, bool, error) {
	return readOwnershipMarker(root)
}
